package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopress/internal/store"
)

type aiTransport func(*http.Request) (*http.Response, error)

func (f aiTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func aiTestService(t *testing.T) *Service {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err = s.SaveUser(context.Background(), store.User{Email: "owner@example.com", Role: "admin"}, []byte("hash")); err != nil {
		t.Fatal(err)
	}
	return &Service{Store: s, attempts: map[string]attempt{}}
}
func TestAISettingsKeepSecretsAndRejectConflicts(t *testing.T) {
	v := aiTestService(t)
	ctx := store.WithActor(context.Background(), store.User{ID: 1, Role: "admin"})
	value := AISettings{Model: "openrouter/auto", Key: "private-test-key"}
	if err := v.SaveAISettings(ctx, value, false); err != nil {
		t.Fatal(err)
	}
	current, err := v.AISettings()
	if err != nil || current.Key != value.Key {
		t.Fatal(current, err)
	}
	info, err := os.Stat(filepath.Join(v.Store.Dir, "ai.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
	if err = v.SaveAISettings(ctx, value, false); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	current.Key = ""
	current.Model = "example/model"
	if err = v.SaveAISettings(ctx, current, false); err != nil {
		t.Fatal(err)
	}
	current, _ = v.AISettings()
	if current.Key != value.Key {
		t.Fatal("empty input erased key")
	}
	if err = v.SaveAISettings(ctx, current, true); err != nil {
		t.Fatal(err)
	}
	current, _ = v.AISettings()
	if current.Key != "" {
		t.Fatal("key not removed")
	}
	current.Model = "bad model"
	if err = v.SaveAISettings(ctx, current, false); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	current.Model = "example/model"
	current.Key = "bad\nkey"
	if err = v.SaveAISettings(ctx, current, false); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if err = v.SaveAISettings(store.WithActor(ctx, store.User{Role: "author"}), current, false); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
}
func TestAIRequestIsBoundedAndUsesServerCredentials(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		valid      bool
	}{
		{"success", `{"choices":[{"finish_reason":"stop","message":{"content":" <script>plain text</script> "}}]}`, 200, true},
		{"key rejected", `{"error":{"message":"private-test-key"}}`, 401, false},
		{"partial", `{"choices":[{"finish_reason":"length","message":{"content":"incomplete"}}]}`, 200, false},
		{"invalid", "not JSON", 200, false},
		{"oversized", strings.Repeat("x", (256<<10)+1), 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: aiTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://openrouter.ai/api/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer private-test-key" {
					t.Fatal("wrong endpoint or credentials")
				}
				var body struct {
					Model     string
					MaxTokens int `json:"max_tokens"`
					Messages  []struct{ Role, Content string }
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body.Model != "example/model" || body.MaxTokens != 4096 || !strings.Contains(body.Messages[0].Content, "pl") || !strings.Contains(body.Messages[1].Content, "Reference") {
					t.Fatal(body)
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			text, err := requestAI(context.Background(), AISettings{Model: "example/model", Key: "private-test-key"}, AIInput{Language: "pl", Prompt: "Write", Text: "Reference"}, client)
			if tc.valid {
				if err != nil || text != "<script>plain text</script>" {
					t.Fatal(text, err)
				}
			} else if !errors.Is(err, ErrAIUnavailable) || strings.Contains(err.Error(), "private-test-key") {
				t.Fatal(text, err)
			}
		})
	}
}
func TestAIAccessValidationAndConcurrentLimit(t *testing.T) {
	v := aiTestService(t)
	ctx := store.WithActor(context.Background(), store.User{ID: 1, Role: "admin", Email: "owner@example.com"})
	input := AIInput{Prompt: "Write a page", Language: "ru"}
	if _, err := v.GenerateAI(context.Background(), input); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := v.GenerateAI(ctx, input); !errors.Is(err, ErrAINotConfigured) {
		t.Fatal(err)
	}
	if err := v.SaveAISettings(ctx, AISettings{Model: "example/model", Key: "test-key"}, false); err != nil {
		t.Fatal(err)
	}
	v.aiRequestMu.Lock()
	if _, err := v.GenerateAI(ctx, input); !errors.Is(err, ErrRateLimit) {
		t.Fatal(err)
	}
	v.aiRequestMu.Unlock()
	originalClient := aiClient
	t.Cleanup(func() { aiClient = originalClient })
	aiClient = &http.Client{Transport: aiTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"content":"Generated text"}}]}`))}, nil
	})}
	for range 2 {
		if text, err := v.GenerateAI(ctx, input); err != nil || text != "Generated text" {
			t.Fatal(text, err)
		}
	}
	if v.attempts["ai:owner@example.com"].Count != 2 {
		t.Fatal("AI quota was not counted")
	}
	input.Text = strings.Repeat("x", 64001)
	if _, err := v.GenerateAI(ctx, input); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	p, err := v.Store.Save(ctx, store.Page{Title: "Private", Slug: "private", Kind: "post", Document: `{"type":"doc","content":[{"type":"paragraph"}]}`}, false)
	if err != nil {
		t.Fatal(err)
	}
	input.Text = ""
	input.ID = p.ID
	author := store.WithActor(ctx, store.User{ID: 2, Role: "author"})
	if _, err = v.GenerateAI(author, input); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
}

package httpui

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"gopress/internal/service"
	"gopress/internal/store"
)

func TestLivenessDoesNotWaitForSiteOperations(t *testing.T) {
	s, app := openApp(t, t.TempDir())
	defer s.Close()
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		app.Echo.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	status(t, get("/healthz?installed=1"), 503)
	if err := app.Service.Install(context.Background(), app.Service.InstallToken, store.Site{Title: "Site", BaseURL: "https://example.com", Timezone: "UTC"}, "owner@example.com", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	app.Service.Operations.Lock()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- get("/healthz?installed=1") }()
	select {
	case w := <-done:
		app.Service.Operations.Unlock()
		status(t, w, 200)
	case <-time.After(time.Second):
		app.Service.Operations.Unlock()
		<-done
		t.Fatal("liveness waited for the site lock")
	}
	status(t, get("/readyz?installed=1"), 200)
}

func TestTemplateHistorySurvivesFailedCommit(t *testing.T) {
	for _, failure := range []string{"working-file", "history-file"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			history := filepath.Join(root, "history.json")
			if err := os.WriteFile(history, []byte("previous A"), 0600); err != nil {
				t.Fatal(err)
			}
			working := filepath.Join(root, "working.html")
			staged, err := stageTemplateFile(working, []byte("new C"))
			if err != nil {
				t.Fatal(err)
			}
			stagedHistory, err := stageTemplateFile(history, []byte("current B"))
			if err != nil {
				t.Fatal(err)
			}
			if failure == "working-file" {
				if err = os.Mkdir(working, 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				if err = os.Remove(stagedHistory); err != nil {
					t.Fatal(err)
				}
			}
			if err = commitTemplateSave(staged, working, stagedHistory, history); err == nil {
				t.Fatal("expected commit failure")
			}
			raw, err := os.ReadFile(history)
			if err != nil || string(raw) != "previous A" {
				t.Fatalf("history lost: %q, %v", raw, err)
			}
		})
	}
}

func TestTemplateSaveChecksFinalPackageSizeAndSkipsBinaryReads(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "theme")
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	initial := map[string]string{"layout.html": `<!doctype html><html><body>{{template "body" .}}</body></html>`, "page.html": `{{define "body"}}<p>Page</p>{{end}}`, "assets/site.css": "body{}"}
	size := 0
	for name, text := range initial {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		size += len(text)
	}
	remaining := (32 << 20) - 1024 - size
	for i := 0; remaining > 0; i++ {
		n := min(remaining, 8<<20)
		f, err := os.Create(filepath.Join(root, "assets", fmt.Sprintf("binary-%d.dat", i)))
		if err != nil {
			t.Fatal(err)
		}
		err = errors.Join(f.Truncate(int64(n)), f.Close())
		if err != nil {
			t.Fatal(err)
		}
		remaining -= n
	}
	app := &App{Service: &service.Service{Store: &store.Store{Dir: dir}}}
	files, _, total, err := app.templateFiles("custom", "assets/site.css", true)
	if err != nil {
		t.Fatal(err)
	}
	if total != (32<<20)-1024 || len(files["assets/binary-0.dat"].Data) != 0 || len(files["page.html"].Data) == 0 {
		t.Fatal("save loading must retain sizes and HTML without binary contents")
	}
	err = app.saveTemplateFile("custom", "assets/site.css", templateFileHash([]byte(initial["assets/site.css"])), strings.Repeat(" ", 2048), false)
	if !errors.Is(err, service.ErrInvalid) {
		t.Fatal("oversized package accepted", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "assets/site.css"))
	if err != nil || string(raw) != initial["assets/site.css"] {
		t.Fatal("failed save changed CSS", err)
	}
	if _, err = os.Stat(app.templateHistoryPath("custom", "assets/site.css")); !os.IsNotExist(err) {
		t.Fatal("invalid save changed history", err)
	}
}

func TestRequestErrorDiagnosticsDoNotExposeSecrets(t *testing.T) {
	secret := "private-api-key"
	err := fmt.Errorf("%s: %w", secret, &os.PathError{Op: "open", Path: "/" + secret, Err: syscall.ENOSPC})
	detail := requestErrorDetail(err)
	if !strings.Contains(detail, "no space") || strings.Contains(detail, secret) {
		t.Fatal(detail)
	}
	if detail = requestErrorDetail(errors.New(secret)); strings.Contains(detail, secret) || !strings.Contains(detail, "errorString") {
		t.Fatal(detail)
	}
	s, openErr := store.Open(t.TempDir())
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer s.Close()
	_, dbErr := s.DB.Exec(`SELECT * FROM "private-api-key"`)
	if dbErr == nil {
		t.Fatal("expected database failure")
	}
	detail = requestErrorDetail(dbErr)
	if !strings.Contains(detail, "database error, code") || strings.Contains(detail, secret) {
		t.Fatal(detail)
	}

}

func TestRepeatedPublicBlocksKeepIndependentFormsAndOneStylesheet(t *testing.T) {
	_, app, _ := seoApp(t)
	ctx := context.Background()
	id, err := app.Service.SaveForm(ctx, store.Form{Name: "Contact", Recipient: "owner@example.com", Button: "Send", Success: "Thanks", Enabled: true, Fields: []store.FormField{{Type: "text", Label: "Name"}}})
	if err != nil {
		t.Fatal(err)
	}
	c := app.Echo.NewContext(httptest.NewRequest("GET", "/", nil), httptest.NewRecorder())
	body := fmt.Sprintf(`<p>[form id=%d]</p><p>[form id=%d]</p>`, id, id)
	out, err := app.renderForms(c, body)
	if err != nil {
		t.Fatal(err)
	}
	keys := regexp.MustCompile(`name="submission_key" value="([^"]+)"`).FindAllStringSubmatch(out, -1)
	if len(keys) != 2 || keys[0][1] == keys[1][1] {
		t.Fatal("repeated forms must retain independent submission keys")
	}
	module := store.ModulePackage{ID: "audit-card", Enabled: true, Manifest: `{"id":"audit-card","name":"Card","version":"1.0","fields":[]}`, Template: `<p>Module content</p>`, Settings: `{}`, CSS: `.gp-module{color:inherit}`}
	if err = app.Service.Store.InstallModulePackage(ctx, module); err != nil {
		t.Fatal(err)
	}
	if err = app.Service.Store.ToggleModulePackage(ctx, "audit-card", 1, true); err != nil {
		t.Fatal(err)
	}
	body = `<p id="section-first">[module id="audit-card"]</p><p id="section-second">[module id="audit-card"]</p>`
	out, err = app.renderModules(c, body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, `href="/module-assets/audit-card/style.css"`) != 1 || strings.Count(out, "Module content") != 2 || !strings.Contains(out, `id="section-first"`) || !strings.Contains(out, `id="section-second"`) {
		t.Fatal(out)
	}
	if err = app.Service.Store.ToggleModulePackage(ctx, "audit-card", 2, false); err != nil {
		t.Fatal(err)
	}
	out, err = app.renderModules(c, body)
	if err != nil || out != "" {
		t.Fatal("render state leaked between requests", out, err)
	}
}

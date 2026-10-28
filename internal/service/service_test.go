package service

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"os"
	"strings"
	"testing"

	"gopress/internal/store"
)

func TestTiptapDocumentContract(t *testing.T) {
	raw, err := os.ReadFile("../../web/testdata/editor-document.json")
	if err != nil {
		t.Fatal(err)
	}
	body, err := Render(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"<h2>Page title</h2>", "<strong>&lt;script&gt;alert(1)&lt;/script&gt;</strong>", "<p></p>"} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("missing %s", fragment)
		}
	}
	if strings.Contains(body, "<script>") {
		t.Fatal("unsafe editor output")
	}
}

func TestRendererRejectsInvalidDocuments(t *testing.T) {
	for _, raw := range []string{`{}`, `{"type":"doc","content":[{"type":"paragraph","attrs":{"onclick":"bad"}}]}`, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link"}]}]}]}`, `{"type":"doc","content":[{"type":"heading","attrs":{"level":9}}]}`, `{"type":"doc","content":[]} {}`, strings.Repeat("x", (1<<20)+1)} {
		if _, err := Render(raw); err == nil {
			t.Fatalf("accepted %s", raw[:min(80, len(raw))])
		}
	}
	raw := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"<script>","marks":[{"type":"bold"},{"type":"italic"}]}]}]}`
	out, err := Render(raw)
	if err != nil || strings.Contains(out, "<script>") || !strings.Contains(out, "<strong>") {
		t.Fatalf("render: %s %v", out, err)
	}
}
func TestRateLimitAndPixelLimit(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if _, err = v.Login("127.0.0.1", "unknown@example.com", "bad"); err != ErrUnauthorized {
			t.Fatal(err)
		}
	}
	if _, err = v.Login("127.0.0.1", "unknown@example.com", "bad"); err != ErrRateLimit {
		t.Fatal(err)
	}
	// Valid PNG IHDR declaring a decompression bomb, rejected before decoding pixels.
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	payload := make([]byte, 13)
	binary.BigEndian.PutUint32(payload, 100000)
	binary.BigEndian.PutUint32(payload[4:], 100000)
	payload[8] = 8
	payload[9] = 2
	binary.Write(&b, binary.BigEndian, uint32(13))
	b.WriteString("IHDR")
	b.Write(payload)
	sum := crc32.ChecksumIEEE(append([]byte("IHDR"), payload...))
	binary.Write(&b, binary.BigEndian, sum)
	if _, err = v.Upload(&b); err != ErrInvalid {
		t.Fatalf("pixel bomb: %v", err)
	}
}

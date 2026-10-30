package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"gopress/internal/backup"
	"gopress/internal/service"
	"gopress/internal/store"
	"gopress/internal/update"
	"gopress/internal/version"
)

func TestUpdateStartupHelper(t *testing.T) {
	if os.Getenv("GOPRESS_UPDATE_STARTUP_TEST") != "1" {
		return
	}
	data := os.Getenv("GOPRESS_UPDATE_STARTUP_DATA")
	if !strings.HasPrefix(filepath.Base(data), "gopress-update-startup-") {
		t.Fatal("invalid fixture directory")
	}
	version.Current = "9.0.0"
	if err := run([]string{"serve", "--data", data, "--addr", "127.0.0.1:0"}); err != nil {
		t.Fatal(err)
	}
}
func TestUpdateStartupCompletesInNewProcess(t *testing.T) {
	data, err := os.MkdirTemp(t.TempDir(), "gopress-update-startup-")
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Install(context.Background(), store.Site{Title: "Update fixture", BaseURL: "https://example.com", Timezone: "UTC"}, "owner@example.com", []byte("hash")); err != nil {
		t.Fatal(err)
	}
	id := "20261030T120000-1234abcd"
	root := filepath.Join(data, "backups")
	if err = os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err = backup.CreateFromStore(s, filepath.Join(root, id)); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	release := update.Release{Format: 1, Version: "9.0.0", MinimumVersion: "0.2.0", Schema: store.SchemaVersion, Image: version.ImageRepository + "@sha256:" + strings.Repeat("a", 64)}
	raw, _ := json.Marshal(release)
	state := service.UpdateState{Phase: "requested", FromVersion: version.Current, Backup: id, Started: time.Now().Unix(), Release: update.Signed{Manifest: raw, Signature: ed25519.Sign(private, raw)}}
	raw, _ = json.Marshal(state)
	if err = os.Mkdir(filepath.Join(data, "updates"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{"updates/state.json": raw, "maintenance": []byte("pending"), "update-in-progress": []byte("9.0.0")} {
		if err = os.WriteFile(filepath.Join(data, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestUpdateStartupHelper$")
	cmd.Env = append(os.Environ(), "GOPRESS_UPDATE_STARTUP_TEST=1", "GOPRESS_UPDATE_STARTUP_DATA="+data, "GOPRESS_UPDATE_PUBLIC_KEY="+base64.StdEncoding.EncodeToString(public), "GOPRESS_SMTP_HOST=", "GOPRESS_SMTP_FROM=", "GOPRESS_TRUSTED_PROXIES=")
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	})
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			done <- err
			t.Fatalf("new version exited before readiness: %v", err)
		default:
		}
		body, err := os.ReadFile(filepath.Join(data, "updates/state.json"))
		if err == nil && json.Unmarshal(body, &state) == nil && state.Phase == "complete" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if state.Phase != "complete" {
		t.Fatal("new process did not verify the pending update")
	}
	for _, name := range []string{"update-in-progress", "maintenance"} {
		if _, err = os.Stat(filepath.Join(data, name)); !os.IsNotExist(err) {
			t.Fatal("update did not release maintenance", name, err)
		}
	}
	if err = backup.Verify(filepath.Join(root, id)); err != nil {
		t.Fatal("pre-update backup changed", err)
	}
}

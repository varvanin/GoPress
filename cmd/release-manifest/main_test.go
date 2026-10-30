package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopress/internal/update"
)

func TestSignProducesVerifiableManifest(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	t.Setenv("GOPRESS_RELEASE_PRIVATE_KEY", base64.StdEncoding.EncodeToString(private))
	root := t.TempDir()
	notes := filepath.Join(root, "notes.txt")
	output := filepath.Join(root, "update.json")
	if err := os.WriteFile(notes, []byte("Release notes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"sign", "--version", "1.0.0", "--minimum-version", "0.2.0", "--schema", "16", "--image", "ghcr.io/varvanin/gopress@sha256:" + strings.Repeat("a", 64), "--notes-file", notes, "--output", output}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(output)
	sig, _ := os.ReadFile(output + ".sig")
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil {
		t.Fatal(err)
	}
	release, err := (update.Signed{Manifest: raw, Signature: signature}).Verify(base64.StdEncoding.EncodeToString(public), "ghcr.io/varvanin/gopress")
	if err != nil || release.Version != "1.0.0" || release.Notes != "Release notes" {
		t.Fatal(release, err)
	}
}
func TestKeygenNeverOverwritesPrivateKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.key")
	if err := run([]string{"keygen", path}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 || len(strings.TrimSpace(string(raw))) != 88 {
		t.Fatal("unsafe private key file")
	}
	if err := run([]string{"keygen", path}); err == nil {
		t.Fatal("overwrote signing key")
	}
	if err := run([]string{"validate-version", "1.0.0-rc1"}); err == nil {
		t.Fatal("accepted prerelease")
	}
}

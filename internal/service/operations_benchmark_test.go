package service

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gopress/internal/store"
)

func BenchmarkBackupList(b *testing.B) {
	dir := b.TempDir()
	for i := 0; i < 10; i++ {
		root := filepath.Join(dir, "backups", fmt.Sprintf("20260101T1200%02d-%08x", i, i), "uploads")
		if err := os.MkdirAll(root, 0700); err != nil {
			b.Fatal(err)
		}
		for j := 0; j < 100; j++ {
			if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("image-%d.png", j)), []byte("fixture"), 0600); err != nil {
				b.Fatal(err)
			}
		}
	}
	v := &Service{Store: &store.Store{Dir: dir}}
	b.ReportAllocs()
	for b.Loop() {
		backups, err := v.Backups()
		if err != nil || len(backups) != 10 {
			b.Fatal(backups, err)
		}
	}
}

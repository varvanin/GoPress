package service

import (
	"context"
	"encoding/base32"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopress/internal/backup"
	"gopress/internal/store"
)

func TestSecondFactorEnrollmentRecoveryAndMissingSecretFailClosed(t *testing.T) {
	v := scheduledBackupService(t)
	ctx := context.Background()
	user, err := v.Store.UserByEmail("owner@example.test")
	if err != nil {
		t.Fatal(err)
	}
	actor := store.WithActor(ctx, user)
	password := "a sufficiently long password"
	token, err := v.Login("first", "owner@example.test", password)
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := v.BeginSecondFactor(actor, password)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enrollment.URI, "otpauth://totp/") || !strings.Contains(enrollment.URI, "period=30") {
		t.Fatal("invalid enrollment URI")
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	if err != nil {
		t.Fatal(err)
	}
	codes, err := v.EnableSecondFactor(actor, password, totpCode(key, time.Now().Unix()/30))
	if err != nil || len(codes) != 10 {
		t.Fatal("enrollment failed", err)
	}
	if _, err = v.Store.SessionUser(Hash(token)); err == nil {
		t.Fatal("activation did not revoke old session")
	}
	if _, err = v.Login("no-code", user.Email, password); err == nil {
		t.Fatal("password-only login bypassed 2FA")
	}
	recoveryToken, err := v.Login("recovery", user.Email, password, LoginOptions{Code: codes[0], UserAgent: "test browser"})
	if err != nil {
		t.Fatal("recovery login failed", err)
	}
	if _, err = v.Login("replay", user.Email, password, LoginOptions{Code: codes[0]}); err == nil {
		t.Fatal("recovery replay succeeded")
	}
	profile, _ := v.Store.SecurityProfile(user.ID)
	if profile.RecoveryCount != 9 {
		t.Fatal("recovery code not consumed", profile.RecoveryCount)
	}
	path, _ := v.factorPath(profile.Identity, false)
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("unsafe credential permissions", err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err = v.Login("missing-key", user.Email, password, LoginOptions{Code: "123456"}); !errors.Is(err, ErrSecondFactorUnavailable) {
		t.Fatal("missing key bypassed or misreported", err)
	}
	currentUser, err := v.Store.SessionUser(Hash(recoveryToken))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = v.ChangeSecondFactor(store.WithActor(ctx, currentUser), password, codes[1], true); err != nil {
		t.Fatal("cannot recover without server key", err)
	}
	if _, err = v.Login("after-disable", user.Email, password); err != nil {
		t.Fatal("disable did not restore password login", err)
	}
}
func TestSecondFactorSecretsStayOutsideOrdinaryBackupsAndRecoverableAfterMove(t *testing.T) {
	v := scheduledBackupService(t)
	ctx := context.Background()
	user, _ := v.Store.UserByEmail("owner@example.test")
	actor := store.WithActor(ctx, user)
	password := "a sufficiently long password"
	setup, err := v.BeginSecondFactor(actor, password)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(setup.Secret)
	codes, err := v.EnableSecondFactor(actor, password, totpCode(key, time.Now().Unix()/30))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(t.TempDir(), "backup")
	if err = backup.CreateFromStore(v.Store, snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(snapshot, "security")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("credential directory included in backup", err)
	}
	restored := filepath.Join(t.TempDir(), "restored")
	if err = backup.Restore(snapshot, restored); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(restored)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	moved, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = moved.Login("moved-password-only", user.Email, password); err == nil {
		t.Fatal("missing factor file disabled 2FA")
	}
	if _, err = moved.Login("moved-recovery", user.Email, password, LoginOptions{Code: codes[0]}); err != nil {
		t.Fatal("recovery after move failed", err)
	}
	if err = moved.ResetSecondFactorOffline(user.Email); err != nil {
		t.Fatal(err)
	}
	if _, err = moved.Login("offline-reset", user.Email, password); err != nil {
		t.Fatal("offline reset did not recover access", err)
	}
}

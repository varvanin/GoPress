package service

import (
	"context"
	"testing"

	"gopress/internal/store"
)

func TestPasswordRecoveryIsSingleUseAndRevokesSessions(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = v.Install(ctx, v.InstallToken, store.Site{Title: "Site", BaseURL: "https://example.com", Timezone: "UTC"}, "owner@example.com", "a sufficiently long password"); err != nil {
		t.Fatal(err)
	}
	session, err := v.Login("first", "owner@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	token := Token()
	if err = s.CreateReset(1, Hash(token)); err != nil {
		t.Fatal(err)
	}
	if err = v.ResetPassword(ctx, "reset", token, "a different long password"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.Authorized(Hash(session)); ok {
		t.Fatal("old session survived reset")
	}
	if err = v.ResetPassword(ctx, "reset", token, "another sufficiently long password"); err != ErrInvalid {
		t.Fatal("token reused", err)
	}
	if _, err = v.Login("old", "owner@example.com", "a sufficiently long password"); err != ErrUnauthorized {
		t.Fatal("old password accepted", err)
	}
	if _, err = v.Login("new", "owner@example.com", "a different long password"); err != nil {
		t.Fatal(err)
	}
	u, _ := s.User(1)
	if err = v.ChangePassword(store.WithActor(ctx, u), "wrong", "yet another long password"); err != ErrUnauthorized {
		t.Fatal("wrong current password accepted", err)
	}
}

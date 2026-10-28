package store

import (
	"context"
	"testing"
)

func TestUsersRejectDuplicateEmailWithoutChangingOriginal(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	id, err := s.SaveUser(ctx, User{Email: "owner@example.com", Role: "admin"}, []byte("hash"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveUser(ctx, User{Email: "OWNER@example.com", Role: "author"}, []byte("other")); err != ErrConflict {
		t.Fatal(err)
	}
	original, err := s.User(id)
	if err != nil || original.Role != "admin" {
		t.Fatal(original, err)
	}
}

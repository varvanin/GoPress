package store

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestSecondFactorBlocksStaleAndUnverifiedSessionIssuance(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	id, err := s.SaveUser(ctx, User{Email: "security@example.test", Role: "admin"}, []byte("hash"))
	if err != nil {
		t.Fatal(err)
	}
	user, _ := s.User(id)
	before, _ := s.SecurityProfile(id)
	counter := int64(100)
	if err = s.ConfigureSecondFactor(WithActor(ctx, user), user, []byte("hash"), before, true, []string{"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, SecondFactorProof{Counter: &counter}); err != nil {
		t.Fatal(err)
	}
	if err = s.LoginSession("stale-password-only", id, []byte("hash"), SessionAuth{Profile: before}); !errors.Is(err, ErrConflict) {
		t.Fatal("issued session across factor activation", err)
	}
	if err = s.LoginSession("missing-proof", id, []byte("hash")); !errors.Is(err, ErrConflict) {
		t.Fatal("legacy caller bypassed 2FA", err)
	}
	profile, _ := s.SecurityProfile(id)
	next := int64(101)
	proof := SessionAuth{Profile: profile, Proof: SecondFactorProof{Counter: &next}}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, reference := range []string{"first", "second"} {
		wg.Add(1)
		go func(reference string) {
			defer wg.Done()
			results <- s.LoginSession(reference, id, []byte("hash"), proof)
		}(reference)
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatal("OTP counter accepted more than once", successes)
	}
	var sessions int
	if err = s.DB.QueryRow(`SELECT count(*) FROM sessions WHERE user_id=?`, id).Scan(&sessions); err != nil || sessions != 1 {
		t.Fatal("unverified sessions persisted", sessions, err)
	}
}
func TestRecoveryCodeConsumptionRollsBackWhenSessionCannotBeIssued(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	id, err := s.SaveUser(ctx, User{Email: "recovery@example.test", Role: "admin"}, []byte("hash"))
	if err != nil {
		t.Fatal(err)
	}
	user, _ := s.User(id)
	profile, _ := s.SecurityProfile(id)
	counter := int64(100)
	recovery := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err = s.ConfigureSecondFactor(ctx, user, []byte("hash"), profile, true, []string{recovery}, SecondFactorProof{Counter: &counter}); err != nil {
		t.Fatal(err)
	}
	profile, _ = s.SecurityProfile(id)
	auth := SessionAuth{Profile: profile, Proof: SecondFactorProof{RecoveryHash: recovery}}
	if err = s.LoginSession("wrong-password", id, []byte("wrong hash"), auth); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err = s.LoginSession("valid-recovery", id, []byte("hash"), auth); err != nil {
		t.Fatal("valid recovery was consumed by a failed login", err)
	}
	if err = s.LoginSession("replay-recovery", id, []byte("hash"), auth); !errors.Is(err, ErrConflict) {
		t.Fatal("recovery code replay accepted", err)
	}
}

func TestSessionRevocationRequiresLiveOwner(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	id, _ := s.SaveUser(ctx, User{Email: "one@example.test", Role: "admin"}, []byte("hash"))
	other, _ := s.SaveUser(ctx, User{Email: "two@example.test", Role: "editor"}, []byte("hash"))
	for reference, owner := range map[string]int64{"current": id, "other": id, "foreign": other} {
		if err = s.LoginSession(reference, owner, []byte("hash")); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.RevokeUserSession(ctx, id, "current", "foreign"); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign session revoked", err)
	}
	if err = s.RevokeUserSession(ctx, id, "expired", "other"); !errors.Is(err, ErrConflict) {
		t.Fatal("stale session could revoke", err)
	}
	if err = s.RevokeOtherSessions(ctx, id, "current"); err != nil {
		t.Fatal(err)
	}
	for _, reference := range []string{"current", "foreign"} {
		if _, err = s.SessionUser(reference); err != nil {
			t.Fatal(reference, err)
		}
	}
	if _, err = s.SessionUser("other"); err == nil {
		t.Fatal("other session remains")
	}
	if err = s.RevokeUserSession(ctx, id, "current", "current"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SessionUser("current"); err == nil {
		t.Fatal("current session remains")
	}
}

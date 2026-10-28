package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestFormDeduplicationQueueLeaseRetryAndRetention(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	id, err := s.SaveForm(ctx, Form{Name: "Contact", Recipient: "owner@example.com", Button: "Send", Success: "Thanks", Enabled: true, Fields: []FormField{{Label: "Message", Type: "text"}}})
	if err != nil {
		t.Fatal(err)
	}
	form, _ := s.Form(id)
	first, err := s.SaveFormEntry(ctx, form, []FormValue{{Label: "Message", Value: "=DANGEROUS()"}}, "same-nonce")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.SaveFormEntry(ctx, form, []FormValue{{Label: "Message", Value: "=DANGEROUS()"}}, "same-nonce")
	if err != nil || first != second {
		t.Fatal("duplicate submission", first, second, err)
	}
	now := time.Now().Unix()
	entry, err := s.ClaimFormEntry(ctx, now)
	if err != nil || entry.ID != first || entry.MailAttempts != 1 {
		t.Fatal(entry, err)
	}
	if _, err = s.ClaimFormEntry(ctx, now); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("job claimed twice", err)
	}
	if err = s.DeleteFormEntries(ctx, id, []int64{first}, now); err != ErrConflict {
		t.Fatal("deleted inflight job", err)
	}
	if err = s.FinishFormEntry(ctx, entry, now, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimFormEntry(ctx, now+1); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("retry ignored backoff", err)
	}
	for attempt := 2; attempt <= 5; attempt++ {
		now += 7200
		entry, err = s.ClaimFormEntry(ctx, now)
		if err != nil || entry.MailAttempts != attempt {
			t.Fatal(entry, err)
		}
		if err = s.FinishFormEntry(ctx, entry, now, false); err != nil {
			t.Fatal(err)
		}
	}
	saved, _ := s.FormEntry(first, id)
	if saved.MailStatus != "failed" {
		t.Fatal("retry limit ignored", saved)
	}
	if err = s.RetryFormEntry(ctx, id, first, now); err != nil {
		t.Fatal(err)
	}
	entry, err = s.ClaimFormEntry(ctx, now)
	if err != nil || entry.MailAttempts != 1 {
		t.Fatal("manual retry", entry, err)
	}
	if err = s.FinishFormEntry(ctx, entry, now, true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("UPDATE form_entries SET created_at=? WHERE id=?", now-100*86400, first); err != nil {
		t.Fatal(err)
	}
	if err = s.PrunePrivateData(ctx, now, 90, 365); err != nil {
		t.Fatal(err)
	}
	if _, err = s.FormEntry(first, id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("retention ignored", err)
	}
}

func TestExpiredFinalMailLeaseBecomesRetryableFailure(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	id, err := s.SaveForm(ctx, Form{Name: "Contact", Recipient: "owner@example.com", Button: "Send", Success: "Thanks", Enabled: true, Fields: []FormField{{Label: "Message", Type: "text"}}})
	if err != nil {
		t.Fatal(err)
	}
	form, err := s.Form(id)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := s.SaveFormEntry(ctx, form, []FormValue{{Label: "Message", Value: "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("UPDATE form_entries SET mail_attempts=5,mail_claim_until=100 WHERE id=?", entry); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimFormEntry(ctx, 101); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	var status string
	if err = s.DB.QueryRow("SELECT mail_status FROM form_entries WHERE id=?", entry).Scan(&status); err != nil || status != "failed" {
		t.Fatal(status, err)
	}
	if err = s.RetryFormEntry(ctx, id, entry, 101); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimFormEntry(ctx, 101); err != nil {
		t.Fatal(err)
	}
}

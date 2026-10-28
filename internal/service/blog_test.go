package service

import (
	"context"
	"testing"
	"time"

	"gopress/internal/store"
)

func TestPublicationTimezone(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	got, err := PublicationTime("2030-01-02T12:00", "Europe/Warsaw", now)
	if err != nil || got != time.Date(2030, 1, 2, 11, 0, 0, 0, time.UTC).Unix() {
		t.Fatalf("timezone %d %v", got, err)
	}
	for _, value := range []string{"2029-01-01T12:00", "2030-03-31T02:30", "2030-10-27T02:30", "invalid"} {
		if _, err = PublicationTime(value, "Europe/Warsaw", now); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
}
func TestTermValidationAndCaseInsensitiveUniqueness(t *testing.T) {
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
	if err = v.SaveTerm(ctx, store.Term{Kind: "category", Name: " Новости  проекта "}, false); err != nil {
		t.Fatal(err)
	}
	if err = v.SaveTerm(ctx, store.Term{Kind: "category", Name: "новости проекта"}, false); err == nil {
		t.Fatal("duplicate Cyrillic name")
	}
	if err = v.SaveTerm(ctx, store.Term{Kind: "unknown", Name: "x"}, false); err == nil {
		t.Fatal("unknown taxonomy")
	}
	terms, err := s.Terms(false)
	if err != nil || len(terms) != 1 || terms[0].Name != "Новости проекта" {
		t.Fatal("normalization")
	}
}
func TestSchedulerStopsOnCancellation(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { v.RunScheduler(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not stop")
	}
}

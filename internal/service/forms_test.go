package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"

	"gopress/internal/store"
)

func TestFormValidationEntriesAndDeliveryFailure(t *testing.T) {
	t.Setenv("GOPRESS_SMTP_HOST", "")
	t.Setenv("GOPRESS_SMTP_FROM", "")
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v := &Service{Store: s, attempts: map[string]attempt{}}
	f := store.Form{Name: "Contact", Recipient: "owner@example.com", Button: "Send", Success: "Thanks", Enabled: true, Fields: []store.FormField{{Label: "Email", Type: "email", Required: true}, {Label: "Topic", Type: "select", Required: true, Options: []string{"Sales", "Support"}}, {Label: "Agree", Type: "checkbox", Required: true}}}
	ctx := context.Background()
	for _, change := range []func(*store.Form){func(f *store.Form) { f.Recipient = "x\r\nBcc: victim@example.com" }, func(f *store.Form) { f.Fields = nil }, func(f *store.Form) { f.Name = "" }, func(f *store.Form) { f.Button = "" }} {
		invalid := f
		change(&invalid)
		if _, err = v.SaveForm(ctx, invalid); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted invalid form: %v", err)
		}
	}
	id, err := v.SaveForm(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	f, err = s.Form(id)
	if err != nil {
		t.Fatal(err)
	}
	values := url.Values{"field_0": {"sender@example.com"}, "field_1": {"Sales"}, "field_2": {"Да"}}
	for key, value := range map[string]string{"field_0": "broken", "field_1": "Injected", "field_2": "no"} {
		bad := url.Values{}
		for k, v := range values {
			bad[k] = v
		}
		bad.Set(key, value)
		if _, err = FormValues(f, bad); err == nil {
			t.Fatalf("accepted invalid %s", key)
		}
	}
	bad := url.Values{}
	if _, err = FormValues(f, bad); err == nil {
		t.Fatal("required fields not enforced")
	}
	result, err := FormValues(f, values)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := s.SaveFormEntry(ctx, f, result)
	if err != nil {
		t.Fatal(err)
	}
	if worked, err := v.ProcessFormMail(ctx, time.Now().Unix()); err != nil || worked {
		t.Fatal("worker tried sending without SMTP", worked, err)
	}
	saved, err := s.FormEntry(entry, id)
	if err != nil || saved.MailStatus != "pending" || saved.Values[0].Value != "sender@example.com" {
		t.Fatal(saved, err)
	}
	old := f
	f.Name = "Changed"
	if _, err = v.SaveForm(ctx, f); err != nil {
		t.Fatal(err)
	}
	if _, err = v.SaveForm(ctx, old); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale form overwrote changes", err)
	}
	if _, err = s.SaveFormEntry(ctx, old, result); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale form submitted", err)
	}
	f, _ = s.Form(id)
	f.Enabled = false
	if _, err = v.SaveForm(ctx, f); err != nil {
		t.Fatal(err)
	}
	f, _ = s.Form(id)
	if _, err = s.SaveFormEntry(ctx, f, result); !errors.Is(err, store.ErrConflict) {
		t.Fatal("disabled form accepted submission", err)
	}
	for range 10 {
		if !v.AllowFormSubmission("one") {
			t.Fatal("limited too early")
		}
	}
	if v.AllowFormSubmission("one") || !v.AllowFormSubmission("two") {
		t.Fatal("invalid IP limit")
	}
	if _, err = FormValues(store.Form{Fields: []store.FormField{{Type: "text"}}}, url.Values{"field_0": {strings.Repeat("a", 4001)}}); err == nil {
		t.Fatal("unbounded field")
	}
}

func TestFormShortcodesStayOutOfExcerpts(t *testing.T) {
	raw := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Before"}]},{"type":"paragraph","content":[{"type":"text","text":"[form id=12]"}]},{"type":"paragraph","content":[{"type":"text","text":"[faq id=2]"}]},{"type":"paragraph","content":[{"type":"text","text":"After"}]}]}`
	if got := Excerpt(raw, 160); got != "Before After" {
		t.Fatal(got)
	}
}

func TestFormMailRequiresTLSBeforeCredentials(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if errors.Is(err, syscall.EPERM) {
		t.Skip("sandbox forbids listening sockets; SMTP transport needs an unrestricted test run")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	t.Setenv("GOPRESS_SMTP_HOST", host)
	t.Setenv("GOPRESS_SMTP_PORT", port)
	t.Setenv("GOPRESS_SMTP_FROM", "sender@example.com")
	t.Setenv("GOPRESS_SMTP_USER", "private-user")
	t.Setenv("GOPRESS_SMTP_PASSWORD", "private-password")
	commands := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			commands <- ""
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(2 * time.Second))
		fmt.Fprint(conn, "220 test SMTP\r\n")
		reader := bufio.NewReader(conn)
		var received strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			received.WriteString(line)
			if strings.HasPrefix(line, "EHLO") {
				fmt.Fprint(conn, "250 localhost\r\n")
			} else {
				fmt.Fprint(conn, "502 TLS unavailable\r\n")
			}
		}
		commands <- received.String()
	}()
	if err = sendMail(context.Background(), envMailSettings(), "owner@example.com", "Contact", "Private submission"); err == nil {
		t.Fatal("sent email without TLS")
	}
	sent := <-commands
	if strings.Contains(sent, "AUTH") || strings.Contains(sent, "MAIL FROM") || strings.Contains(sent, "Private") {
		t.Fatal("sent credentials or data before TLS")
	}
}

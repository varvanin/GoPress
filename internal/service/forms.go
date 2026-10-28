package service

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gopress/internal/store"
)

func validAddress(value string) bool {
	a, err := mail.ParseAddress(value)
	return err == nil && a.Address == value && len(value) <= 254 && !strings.ContainsAny(value, "\r\n")
}
func (v *Service) SaveForm(ctx context.Context, f store.Form) (int64, error) {
	f.Name = strings.TrimSpace(f.Name)
	f.Recipient = strings.TrimSpace(f.Recipient)
	f.Button = strings.TrimSpace(f.Button)
	f.Success = strings.TrimSpace(f.Success)
	if !utf8.ValidString(f.Name+f.Recipient+f.Button+f.Success) || f.ID < 0 || f.Version < 0 || f.ID > 0 && f.Version < 1 || f.Name == "" || utf8.RuneCountInString(f.Name) > 120 || !validAddress(f.Recipient) || f.Button == "" || utf8.RuneCountInString(f.Button) > 80 || f.Success == "" || utf8.RuneCountInString(f.Success) > 500 || len(f.Fields) < 1 || len(f.Fields) > 20 {
		return 0, ErrInvalid
	}
	for i := range f.Fields {
		field := &f.Fields[i]
		field.Label = strings.TrimSpace(field.Label)
		if !utf8.ValidString(field.Label) || field.Label == "" || utf8.RuneCountInString(field.Label) > 120 {
			return 0, ErrInvalid
		}
		switch field.Type {
		case "text", "email", "tel", "textarea", "checkbox":
			if len(field.Options) > 0 {
				return 0, ErrInvalid
			}
		case "select":
			if len(field.Options) < 1 || len(field.Options) > 30 {
				return 0, ErrInvalid
			}
			seen := map[string]bool{}
			for j, option := range field.Options {
				option = strings.TrimSpace(option)
				if !utf8.ValidString(option) || option == "" || utf8.RuneCountInString(option) > 120 || seen[option] {
					return 0, ErrInvalid
				}
				seen[option] = true
				field.Options[j] = option
			}
		default:
			return 0, ErrInvalid
		}
	}
	return v.Store.SaveForm(ctx, f)
}
func FormValues(f store.Form, values url.Values) ([]store.FormValue, error) {
	result := make([]store.FormValue, 0, len(f.Fields))
	for i, field := range f.Fields {
		entries := values["field_"+strconv.Itoa(i)]
		if len(entries) > 1 {
			return nil, ErrInvalid
		}
		value := strings.TrimSpace(values.Get("field_" + strconv.Itoa(i)))
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 4000 || strings.ContainsRune(value, 0) || field.Required && value == "" {
			return nil, ErrInvalid
		}
		if field.Type == "email" && value != "" && !validAddress(value) {
			return nil, ErrInvalid
		}
		if field.Type == "checkbox" && value != "" && value != "Да" {
			return nil, ErrInvalid
		}
		if field.Type == "select" && value != "" {
			found := false
			for _, option := range field.Options {
				if value == option {
					found = true
				}
			}
			if !found {
				return nil, ErrInvalid
			}
		}
		result = append(result, store.FormValue{Label: field.Label, Value: value})
	}
	return result, nil
}
func (v *Service) AllowFormSubmission(ip string) bool { return v.allowAttempt("form:"+ip, 10) }

func (v *Service) sendFormEntry(ctx context.Context, settings MailSettings, f store.Form, entry store.FormEntry) error {
	var body strings.Builder
	fmt.Fprintf(&body, "Новая заявка: %s\r\nНомер: %d\r\n\r\n", f.Name, entry.ID)
	for _, value := range entry.Values {
		fmt.Fprintf(&body, "%s: %s\r\n\r\n", value.Label, strings.ReplaceAll(strings.ReplaceAll(value.Value, "\r\n", "\n"), "\n", "\r\n"))
	}
	return sendMail(ctx, settings, f.Recipient, "Заявка: "+f.Name, body.String())
}

func sendMail(ctx context.Context, settings MailSettings, recipient, subject, body string) error {
	if !settings.Configured() {
		return errors.New("SMTP is not configured")
	}
	host, port, from := settings.Host, settings.Port, settings.From
	if port == "" {
		port = "587"
	}
	if !validAddress(recipient) {
		return ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	if err = conn.SetDeadline(deadline); err != nil {
		return err
	}
	config := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	if port == "465" {
		secure := tls.Client(conn, config)
		if err = secure.HandshakeContext(ctx); err != nil {
			return err
		}
		conn = secure
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if port != "465" {
		if err = client.StartTLS(config); err != nil {
			return err
		}
	}
	if user := settings.Username; user != "" {
		if err = client.Auth(smtp.PlainAuth("", user, settings.Password, host)); err != nil {
			return err
		}
	}
	if err = client.Mail(from); err != nil {
		return err
	}
	if err = client.Rcpt(recipient); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s", from, recipient, mime.QEncoding.Encode("UTF-8", subject), body)
	if err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

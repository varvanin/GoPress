package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopress/internal/store"
)

type MailSettings struct {
	Version     string `json:"version"`
	Enabled     bool   `json:"enabled"`
	Host        string `json:"host"`
	Port        string `json:"port"`
	From        string `json:"from"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Environment bool   `json:"-"`
}

func envMailSettings() MailSettings {
	port := os.Getenv("GOPRESS_SMTP_PORT")
	if port == "" {
		port = "587"
	}
	return MailSettings{Version: "environment", Enabled: true, Environment: true, Host: os.Getenv("GOPRESS_SMTP_HOST"), Port: port, From: os.Getenv("GOPRESS_SMTP_FROM"), Username: os.Getenv("GOPRESS_SMTP_USER"), Password: os.Getenv("GOPRESS_SMTP_PASSWORD")}
}
func (m MailSettings) Configured() bool { return m.Enabled && m.Host != "" && validAddress(m.From) }
func (v *Service) MailSettings() (MailSettings, error) {
	if os.Getenv("GOPRESS_SMTP_HOST") != "" || os.Getenv("GOPRESS_SMTP_FROM") != "" {
		return envMailSettings(), nil
	}
	raw, err := os.ReadFile(filepath.Join(v.Store.Dir, "mail.json"))
	if errors.Is(err, os.ErrNotExist) {
		return MailSettings{Port: "587"}, nil
	}
	if err != nil {
		return MailSettings{}, err
	}
	var value MailSettings
	err = json.Unmarshal(raw, &value)
	return value, err
}
func (v *Service) SaveMailSettings(value MailSettings, clearPassword bool) error {
	v.mailConfigMu.Lock()
	defer v.mailConfigMu.Unlock()
	current, err := v.MailSettings()
	if err != nil {
		return err
	}
	if current.Environment {
		return ErrInvalid
	}
	if current.Version != value.Version {
		return store.ErrConflict
	}
	value.Host = strings.TrimSpace(value.Host)
	value.From = strings.TrimSpace(value.From)
	value.Username = strings.TrimSpace(value.Username)
	if value.Port == "" {
		value.Port = "587"
	}
	port, err := strconv.Atoi(value.Port)
	if err != nil || port < 1 || port > 65535 || len(value.Host) > 253 || strings.ContainsAny(value.Host, "/\\:@ \t\r\n") || len(value.Username) > 254 || strings.ContainsAny(value.Username, "\r\n") || len(value.Password) > 4096 || value.Enabled && (value.Host == "" || !validAddress(value.From)) {
		return ErrInvalid
	}
	if value.Password == "" {
		value.Password = current.Password
	}
	if clearPassword {
		value.Password = ""
	}
	value.Version = Token()
	value.Environment = false
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(v.Store.Dir, ".mail-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(raw); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(v.Store.Dir, "mail.json"))
}
func (v *Service) SendTestMail(ctx context.Context, recipient string) error {
	if !validAddress(recipient) {
		return ErrInvalid
	}
	value, err := v.MailSettings()
	if err != nil {
		return err
	}
	return sendMail(ctx, value, recipient, "GoPress — проверка почты", "Тестовое письмо GoPress. Настройки исходящей почты работают.\r\nGoPress test message: outgoing email is working.\r\n")
}

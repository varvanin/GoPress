package service

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"gopress/internal/store"
)

var ErrForbidden = errors.New("forbidden")

func RequireAdmin(ctx context.Context) error {
	if u, ok := store.Actor(ctx); ok && u.Role != "admin" {
		return ErrForbidden
	}
	return nil
}
func CanPublish(ctx context.Context) bool {
	u, ok := store.Actor(ctx)
	return !ok || u.Role == "admin" || u.Role == "editor"
}
func CanEdit(ctx context.Context, p store.Page) bool {
	u, ok := store.Actor(ctx)
	return !ok || u.Role == "admin" || u.Role == "editor" || u.Role == "author" && p.Kind == "post" && p.OwnerID == u.ID
}
func (v *Service) SaveUser(ctx context.Context, u store.User, password string) (int64, error) {
	if err := RequireAdmin(ctx); err != nil {
		return 0, err
	}
	u.Email = strings.ToLower(strings.TrimSpace(u.Email))
	u.Name = strings.TrimSpace(u.Name)
	if u.ID < 0 || u.Version < 0 || u.ID > 0 && u.Version < 1 || !validAddress(u.Email) || !utf8.ValidString(u.Name) || utf8.RuneCountInString(u.Name) > 120 || (u.Role != "admin" && u.Role != "editor" && u.Role != "author") || u.ID == 0 && password == "" {
		return 0, ErrInvalid
	}
	var hash []byte
	var err error
	if password != "" {
		if len(password) < 12 || len(password) > 72 {
			return 0, ErrInvalid
		}
		hash, err = bcrypt.GenerateFromPassword([]byte(password), 12)
		if err != nil {
			return 0, err
		}
	}
	return v.Store.SaveUser(ctx, u, hash)
}
func (v *Service) ChangePassword(ctx context.Context, oldPassword, password string) error {
	u, ok := store.Actor(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if len(password) < 12 || len(password) > 72 {
		return ErrInvalid
	}
	hash, err := v.Store.Password(u.Email)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(oldPassword)) != nil {
		return ErrUnauthorized
	}
	next, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	return v.Store.ChangePassword(ctx, u.ID, hash, next)
}
func (v *Service) allowAttempt(key string, limit int) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := time.Now()
	for key, value := range v.attempts {
		if now.After(value.Until) {
			delete(v.attempts, key)
		}
	}
	a := v.attempts[key]
	if a.Count >= limit || a.Count == 0 && len(v.attempts) >= 4096 {
		return false
	}
	if a.Count == 0 {
		a.Until = now.Add(15 * time.Minute)
	}
	a.Count++
	v.attempts[key] = a
	return true
}
func (v *Service) RequestPasswordReset(ctx context.Context, ip, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if !v.allowAttempt("reset:"+ip, 5) {
		return ErrRateLimit
	}
	// Always perform comparable password work, whether an address exists or not.
	bcrypt.CompareHashAndPassword(v.dummy, []byte("reset-request"))
	u, err := v.Store.UserByEmail(email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	token := Token()
	if err = v.Store.CreateReset(u.ID, Hash(token)); err != nil {
		return err
	}
	site, err := v.Store.Site()
	if err != nil {
		return err
	}
	settings, err := v.MailSettings()
	if err != nil {
		return err
	}
	select {
	case v.resetSlots <- struct{}{}:
	default:
		return v.Store.Audit(ctx, "user.reset_delivery_failed", strconv.FormatInt(u.ID, 10))
	}
	v.Background.Add(1)
	go func() {
		defer v.Background.Done()
		defer func() { <-v.resetSlots }()
		v.Operations.RLock()
		defer v.Operations.RUnlock()
		deliveryCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		action := "user.reset_requested"
		if err := sendMail(deliveryCtx, settings, u.Email, "GoPress — восстановление доступа", "Ссылка действительна 30 минут. / Password reset link (valid for 30 minutes):\r\n\r\n"+site.BaseURL+"/reset-password?token="+token+"\r\n"); err != nil {
			action = "user.reset_delivery_failed"
		}
		v.Store.Audit(context.Background(), action, strconv.FormatInt(u.ID, 10))
	}()
	return nil
}
func (v *Service) ResetPassword(ctx context.Context, ip, token, password string) error {
	if !v.allowAttempt("reset-use:"+ip, 10) {
		return ErrRateLimit
	}
	if len(token) != 64 || len(password) < 12 || len(password) > 72 {
		return ErrInvalid
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	err = v.Store.ResetPassword(ctx, Hash(token), hash)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalid
	}
	return err
}

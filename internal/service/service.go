package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"gopress/internal/store"
)

const MaxDocumentSize = 1 << 20
const MaxContentSize = 4 << 20

// JSON escaping and form encoding can expand the bounded documents sixfold.
const MaxContentRequestSize = 32 << 20

var ErrInvalid = errors.New("invalid input")
var ErrUnauthorized = errors.New("invalid credentials")
var ErrRateLimit = errors.New("too many attempts; retry later")
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type attempt struct {
	Count int
	Until time.Time
}
type Service struct {
	Store        *store.Store
	Operations   sync.RWMutex
	Background   sync.WaitGroup
	resetSlots   chan struct{}
	InstallToken string
	mu           sync.Mutex
	attempts     map[string]attempt
	dummy        []byte
	uploadMu     sync.Mutex
	mediaMu      sync.RWMutex
	mailConfigMu sync.Mutex
}

func New(s *store.Store) (*Service, error) {
	dummy, err := bcrypt.GenerateFromPassword([]byte(Token()), 12)
	if err != nil {
		return nil, err
	}
	v := &Service{Store: s, resetSlots: make(chan struct{}, 4), attempts: map[string]attempt{}, dummy: dummy}
	if _, err = s.Site(); errors.Is(err, sql.ErrNoRows) {
		v.InstallToken = Token()
	} else if err != nil {
		return nil, err
	}
	return v, nil
}
func Token() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func Hash(s string) string   { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
func Equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
func (v *Service) Install(ctx context.Context, token string, site store.Site, email, password string) error {
	if v.InstallToken == "" || !Equal(token, v.InstallToken) {
		return ErrUnauthorized
	}
	site.Title = strings.TrimSpace(site.Title)
	email = strings.ToLower(strings.TrimSpace(email))
	u, err := url.Parse(site.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || strings.ContainsAny(site.BaseURL, "\r\n") {
		return fmt.Errorf("%w: base URL must be an http(s) origin", ErrInvalid)
	}
	site.BaseURL = strings.TrimRight(site.BaseURL, "/")
	if _, err = time.LoadLocation(site.Timezone); err != nil {
		return fmt.Errorf("%w: timezone", ErrInvalid)
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || len(email) > 254 || len(site.Title) == 0 || len(site.Title) > 200 || len(password) < 12 || len(password) > 72 {
		return fmt.Errorf("%w: title, email or password (12–72 bytes)", ErrInvalid)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	return v.Store.Install(ctx, site, email, hash)
}
func (v *Service) Login(ip, email, password string) (string, error) {
	if !v.allowAttempt("login:"+ip, 5) {
		return "", ErrRateLimit
	}
	h, err := v.Store.Password(strings.ToLower(strings.TrimSpace(email)))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if errors.Is(err, sql.ErrNoRows) {
		h = v.dummy
	}
	compare := bcrypt.CompareHashAndPassword(h, []byte(password))
	if err != nil || compare != nil {
		v.Store.Audit(context.Background(), "user.login_failed", "")
		return "", ErrUnauthorized
	}
	u, err := v.Store.UserByEmail(strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return "", err
	}
	token := Token()
	if err = v.Store.LoginSession(Hash(token), u.ID, h); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return "", ErrUnauthorized
		}
		return "", err
	}
	v.Store.Audit(store.WithActor(context.Background(), u), "user.login", "")
	return token, nil
}
func (v *Service) Save(ctx context.Context, p store.Page, publish bool) (store.Page, error) {
	if p.ID > 0 {
		current, err := v.Store.Page(p.ID)
		if err != nil {
			return p, err
		}
		p.Kind = current.Kind
		if !CanEdit(ctx, current) {
			return p, ErrForbidden
		}
	} else if u, ok := store.Actor(ctx); ok && u.Role == "author" && p.Kind != "post" {
		return p, ErrForbidden
	}
	if publish && !CanPublish(ctx) {
		return p, ErrForbidden
	}
	v.mediaMu.RLock()
	defer v.mediaMu.RUnlock()
	if p.Kind != "" && p.Kind != "page" && p.Kind != "post" {
		return p, ErrInvalid
	}
	if p.SEO.Language == "" {
		p.SEO.Language = "ru"
	}
	if !contentLanguage.MatchString(p.SEO.Language) {
		return p, ErrInvalid
	}
	if err := cleanSEO(&p.SEO); err != nil {
		return p, err
	}
	p.Title = strings.TrimSpace(p.Title)
	if p.ID < 0 || len(p.Title) == 0 || !utf8.ValidString(p.Title) || utf8.RuneCountInString(p.Title) > 200 || len(p.Slug) > 1024 || !validContentPath(p.Slug) || p.Kind == "post" && strings.Contains(p.Slug, "/") {
		return p, ErrInvalid
	}
	if store.ReservedContentPath(p.Slug) {
		return p, fmt.Errorf("%w: reserved URL", ErrInvalid)
	}
	if p.Template == "" {
		p.Template = "page.html"
	}
	if p.ParentID < 0 || p.ParentID == p.ID && p.ID > 0 || p.Order < -100000 || p.Order > 100000 || !ValidPageTemplate(p.Template) || p.Kind == "post" && (p.ParentID != 0 || p.Template != "page.html") {
		return p, ErrInvalid
	}
	checkMedia := func(name string) error {
		if u, ok := store.Actor(ctx); ok && u.Role == "author" {
			owner, err := v.Store.MediaOwner(name)
			if err != nil {
				return err
			}
			if owner != u.ID {
				return ErrForbidden
			}
		}
		_, err := v.Store.MediaType(name)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalid
		}
		return err
	}
	if _, err := render(p.Document, checkMedia); err != nil {
		return p, err
	}
	if len(p.Translations) > 20 {
		return p, ErrInvalid
	}
	total := len(p.Document)
	for language, translation := range p.Translations {
		total += len(translation.Document)
		translation.Title = strings.TrimSpace(translation.Title)
		if !contentLanguage.MatchString(language) || !utf8.ValidString(translation.Title) || utf8.RuneCountInString(translation.Title) > 200 || total > MaxContentSize {
			return p, ErrInvalid
		}
		if err := cleanSEO(&translation.SEO); err != nil {
			return p, err
		}
		if _, err := render(translation.Document, checkMedia); err != nil {
			return p, err
		}
		p.Translations[language] = translation
	}
	if p.Cover != "" {
		if u, ok := store.Actor(ctx); ok && u.Role == "author" {
			owner, err := v.Store.MediaOwner(p.Cover)
			if err != nil {
				return p, err
			}
			if owner != u.ID {
				return p, ErrForbidden
			}
		}
		if _, err := v.Store.MediaType(p.Cover); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return p, ErrInvalid
			}
			return p, err
		}
	}
	if len(p.TermIDs) > 100 {
		return p, ErrInvalid
	}
	saved, err := v.Store.Save(ctx, p, publish)
	if errors.Is(err, store.ErrInvalidTerms) {
		return p, ErrInvalid
	}
	return saved, err
}

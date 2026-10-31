package service

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gopress/internal/store"
)

var ErrSecondFactorUnavailable = errors.New("second-factor key unavailable")

type factorFile struct {
	Identity, Secret                     string
	Expires, UserVersion, ProfileVersion int64
}
type TwoFactorStatus struct {
	Enabled, Pending, Unavailable bool
	RecoveryCount                 int
}
type FactorEnrollment struct{ Secret, URI string }

func (v *Service) factorPath(identity string, pending bool) (string, error) {
	if len(identity) != 32 {
		return "", ErrSecondFactorUnavailable
	}
	if _, err := hex.DecodeString(identity); err != nil {
		return "", ErrSecondFactorUnavailable
	}
	suffix := ".json"
	if pending {
		suffix = ".pending.json"
	}
	return filepath.Join(v.Store.Dir, "security", identity+suffix), nil
}
func (v *Service) readFactor(profile store.SecurityProfile, pending bool) (factorFile, error) {
	name, err := v.factorPath(profile.Identity, pending)
	if err != nil {
		return factorFile{}, err
	}
	parent, err := os.Lstat(filepath.Dir(name))
	if err != nil {
		return factorFile{}, err
	}
	if !parent.IsDir() || parent.Mode().Perm()&0077 != 0 {
		return factorFile{}, ErrSecondFactorUnavailable
	}
	info, err := os.Lstat(name)
	if err != nil {
		return factorFile{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16<<10 {
		return factorFile{}, ErrSecondFactorUnavailable
	}
	file, err := os.Open(name)
	if err != nil {
		return factorFile{}, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 16<<10+1))
	if err != nil {
		return factorFile{}, err
	}
	var value factorFile
	if len(raw) > 16<<10 || decodeStrict(string(raw), &value) != nil || value.Identity != profile.Identity {
		return value, ErrSecondFactorUnavailable
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(value.Secret)
	if err != nil || len(key) != 20 {
		return value, ErrSecondFactorUnavailable
	}
	return value, nil
}
func (v *Service) writeFactor(profile store.SecurityProfile, pending bool, value factorFile) error {
	name, err := v.factorPath(profile.Identity, pending)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		return err
	}
	info, e := os.Lstat(filepath.Dir(name))
	if e != nil {
		return e
	}
	if !info.IsDir() {
		return ErrSecondFactorUnavailable
	}
	if err = os.Chmod(filepath.Dir(name), 0700); err != nil {
		return err
	}
	if err = syncDirectory(v.Store.Dir); err != nil {
		return err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(name), ".factor-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(raw)
	}
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	if err = os.Rename(file.Name(), name); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(name))
}
func (v *Service) ownPassword(ctx context.Context, password string) (store.User, []byte, error) {
	actor, ok := store.Actor(ctx)
	if !ok || len(password) == 0 || len(password) > 72 {
		return store.User{}, nil, ErrUnauthorized
	}
	user, err := v.Store.User(actor.ID)
	if err != nil {
		return user, nil, err
	}
	if user.Blocked || user.Version != actor.Version {
		return user, nil, ErrUnauthorized
	}
	hash, err := v.Store.Password(user.Email)
	if err != nil {
		return user, nil, err
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil {
		return user, nil, ErrUnauthorized
	}
	return user, hash, nil
}
func (v *Service) SecondFactorStatus(ctx context.Context) (TwoFactorStatus, error) {
	user, ok := store.Actor(ctx)
	if !ok {
		return TwoFactorStatus{}, ErrUnauthorized
	}
	profile, err := v.Store.SecurityProfile(user.ID)
	if err != nil {
		return TwoFactorStatus{}, err
	}
	status := TwoFactorStatus{Enabled: profile.Enabled, RecoveryCount: profile.RecoveryCount}
	if profile.Enabled {
		_, err = v.readFactor(profile, false)
		status.Unavailable = err != nil
		return status, nil
	}
	pending, err := v.readFactor(profile, true)
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return status, nil
	}
	status.Pending = pending.Expires > time.Now().Unix() && pending.UserVersion == user.Version && pending.ProfileVersion == profile.Version
	return status, nil
}
func (v *Service) BeginSecondFactor(ctx context.Context, password string) (FactorEnrollment, error) {
	v.secondFactorMu.Lock()
	defer v.secondFactorMu.Unlock()
	user, _, err := v.ownPassword(ctx, password)
	if err != nil {
		return FactorEnrollment{}, err
	}
	if !v.allowAttempt("2fa-setup:"+strconv.FormatInt(user.ID, 10), 5) {
		return FactorEnrollment{}, ErrRateLimit
	}
	profile, err := v.Store.SecurityProfile(user.ID)
	if err != nil {
		return FactorEnrollment{}, err
	}
	if profile.Enabled {
		return FactorEnrollment{}, store.ErrConflict
	}
	var key [20]byte
	if _, err = rand.Read(key[:]); err != nil {
		return FactorEnrollment{}, err
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(key[:])
	pending := factorFile{Identity: profile.Identity, Secret: secret, Expires: time.Now().Add(10 * time.Minute).Unix(), UserVersion: user.Version, ProfileVersion: profile.Version}
	if err = v.writeFactor(profile, true, pending); err != nil {
		return FactorEnrollment{}, err
	}
	site, err := v.Store.Site()
	if err != nil {
		return FactorEnrollment{}, err
	}
	issuer := "GoPress (" + strings.ReplaceAll(strings.Join(strings.Fields(site.Title), " "), ":", " ") + ")"
	query := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return FactorEnrollment{Secret: secret, URI: "otpauth://totp/" + url.PathEscape(issuer+":"+user.Email) + "?" + query.Encode()}, nil
}
func recoveryCodes() ([]string, []string) {
	codes, hashes := make([]string, 10), make([]string, 10)
	for i := range codes {
		raw := Token()[:32]
		hashes[i] = Hash(raw)
		codes[i] = raw[:8] + "-" + raw[8:16] + "-" + raw[16:24] + "-" + raw[24:]
	}
	return codes, hashes
}
func (v *Service) factorProof(profile store.SecurityProfile, code string) (store.SecondFactorProof, error) {
	if len(code) > 64 {
		return store.SecondFactorProof{}, ErrUnauthorized
	}
	code = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
	if len(code) == 32 {
		if _, err := hex.DecodeString(code); err != nil {
			return store.SecondFactorProof{}, ErrUnauthorized
		}
		return store.SecondFactorProof{RecoveryHash: Hash(code)}, nil
	}
	if len(code) != 6 {
		return store.SecondFactorProof{}, ErrUnauthorized
	}
	value, err := v.readFactor(profile, false)
	if err != nil {
		return store.SecondFactorProof{}, ErrSecondFactorUnavailable
	}
	counter, ok := verifyTOTP(value.Secret, code, time.Now().Unix(), profile.LastCounter)
	if !ok {
		return store.SecondFactorProof{}, ErrUnauthorized
	}
	return store.SecondFactorProof{Counter: &counter}, nil
}
func (v *Service) EnableSecondFactor(ctx context.Context, password, code string) ([]string, error) {
	v.secondFactorMu.Lock()
	defer v.secondFactorMu.Unlock()
	user, hash, err := v.ownPassword(ctx, password)
	if err != nil {
		return nil, err
	}
	if !v.allowAttempt("2fa-setup:"+strconv.FormatInt(user.ID, 10), 5) {
		return nil, ErrRateLimit
	}
	profile, err := v.Store.SecurityProfile(user.ID)
	if err != nil {
		return nil, err
	}
	if profile.Enabled {
		return nil, store.ErrConflict
	}
	pending, err := v.readFactor(profile, true)
	if err != nil {
		return nil, ErrInvalid
	}
	if pending.Expires <= time.Now().Unix() || pending.UserVersion != user.Version || pending.ProfileVersion != profile.Version {
		return nil, store.ErrConflict
	}
	counter, ok := verifyTOTP(pending.Secret, strings.TrimSpace(code), time.Now().Unix(), -1)
	if !ok {
		return nil, ErrUnauthorized
	}
	codes, hashes := recoveryCodes()
	active := factorFile{Identity: profile.Identity, Secret: pending.Secret}
	if err = v.writeFactor(profile, false, active); err != nil {
		return nil, err
	}
	if err = v.Store.ConfigureSecondFactor(ctx, user, hash, profile, true, hashes, store.SecondFactorProof{Counter: &counter}); err != nil {
		return nil, err
	}
	name, _ := v.factorPath(profile.Identity, true)
	if e := os.Remove(name); e != nil && !errors.Is(e, os.ErrNotExist) {
		slog.Error("2FA pending key cleanup failed", "user_id", user.ID, "error", e)
	}
	return codes, nil
}
func (v *Service) ChangeSecondFactor(ctx context.Context, password, code string, disable bool) ([]string, error) {
	v.secondFactorMu.Lock()
	defer v.secondFactorMu.Unlock()
	user, hash, err := v.ownPassword(ctx, password)
	if err != nil {
		return nil, err
	}
	profile, err := v.Store.SecurityProfile(user.ID)
	if err != nil {
		return nil, err
	}
	if !profile.Enabled {
		return nil, store.ErrConflict
	}
	if !v.allowAttempt("2fa-manage:"+strconv.FormatInt(user.ID, 10), 5) {
		return nil, ErrRateLimit
	}
	proof, err := v.factorProof(profile, code)
	if err != nil {
		return nil, err
	}
	var codes, hashes []string
	if !disable {
		codes, hashes = recoveryCodes()
	}
	if err = v.Store.ConfigureSecondFactor(ctx, user, hash, profile, !disable, hashes, proof); err != nil {
		return nil, err
	}
	if disable {
		for _, pending := range []bool{false, true} {
			name, _ := v.factorPath(profile.Identity, pending)
			if e := os.Remove(name); e != nil && !errors.Is(e, os.ErrNotExist) {
				slog.Error("2FA disabled key cleanup failed", "user_id", user.ID, "error", e)
			}
		}
	}
	return codes, nil
}
func (v *Service) ResetSecondFactorOffline(email string) error {
	user, err := v.Store.UserByEmail(strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return err
	}
	profile, err := v.Store.SecurityProfile(user.ID)
	if err != nil {
		return err
	}
	if err = v.Store.ResetSecondFactorOffline(user.ID); err != nil {
		return err
	}
	var cleanup error
	for _, pending := range []bool{false, true} {
		name, _ := v.factorPath(profile.Identity, pending)
		if err = os.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanup = errors.Join(cleanup, err)
		}
	}
	return cleanup
}

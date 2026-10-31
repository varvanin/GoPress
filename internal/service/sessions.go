package service

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"strconv"

	"gopress/internal/store"
)

func (v *Service) sessionActor(ctx context.Context, current string) (store.User, error) {
	actor, ok := store.Actor(ctx)
	if !ok {
		return store.User{}, ErrUnauthorized
	}
	user, err := v.Store.SessionUser(current)
	if errors.Is(err, sql.ErrNoRows) {
		return user, ErrUnauthorized
	}
	if err != nil {
		return user, err
	}
	if user.ID != actor.ID {
		return user, ErrForbidden
	}
	return user, nil
}
func (v *Service) RevokeSession(ctx context.Context, current, reference string) error {
	if len(reference) != 64 {
		return ErrInvalid
	}
	if _, err := hex.DecodeString(reference); err != nil {
		return ErrInvalid
	}
	user, err := v.sessionActor(ctx, current)
	if err != nil {
		return err
	}
	if err = v.Store.RevokeUserSession(ctx, user.ID, current, reference); err != nil {
		return err
	}
	return v.Store.Audit(ctx, "user.session_revoked", strconv.FormatInt(user.ID, 10))
}
func (v *Service) RevokeOtherSessions(ctx context.Context, current string) error {
	user, err := v.sessionActor(ctx, current)
	if err != nil {
		return err
	}
	if err = v.Store.RevokeOtherSessions(ctx, user.ID, current); err != nil {
		return err
	}
	return v.Store.Audit(ctx, "user.sessions_revoked", strconv.FormatInt(user.ID, 10))
}

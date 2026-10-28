package service

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"gopress/internal/store"
)

func (v *Service) SaveTerm(ctx context.Context, t store.Term, remove bool) error {
	t.Name = strings.Join(strings.Fields(t.Name), " ")
	if t.ID < 0 || t.ID == 0 && (remove || t.Version != 0) || t.ID > 0 && t.Version < 1 {
		return ErrInvalid
	}
	if !remove && (t.Name == "" || utf8.RuneCountInString(t.Name) > 80 || t.Kind != "category" && t.Kind != "tag") {
		return ErrInvalid
	}
	return v.Store.SaveTerm(ctx, t, remove)
}
func PublicationTime(value, timezone string, now time.Time) (int64, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return 0, ErrInvalid
	}
	at, err := time.ParseInLocation("2006-01-02T15:04", value, loc)
	// Reject nonexistent local times during a spring daylight-saving transition.
	if err != nil || at.Format("2006-01-02T15:04") != value || !at.After(now) {
		return 0, ErrInvalid
	}
	// A repeated local time during autumn has two instants; require an unambiguous choice.
	_, chosenOffset := at.Zone()
	for _, delta := range []time.Duration{-24 * time.Hour, 24 * time.Hour} {
		_, otherOffset := at.Add(delta).Zone()
		if otherOffset != chosenOffset && at.Add(time.Duration(chosenOffset-otherOffset)*time.Second).In(loc).Format("2006-01-02T15:04") == value {
			return 0, ErrInvalid
		}
	}
	return at.Unix(), nil
}
func (v *Service) Schedule(ctx context.Context, id, version int64, value string) error {
	if !CanPublish(ctx) {
		return ErrForbidden
	}
	site, err := v.Store.Site()
	if err != nil {
		return err
	}
	at, err := PublicationTime(value, site.Timezone, time.Now())
	if err != nil {
		return err
	}
	return v.Store.Schedule(ctx, id, version, at)
}
func (v *Service) RunScheduler(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		for {
			v.Operations.RLock()
			active, err := v.Maintenance()
			n := 0
			if err == nil && !active {
				n, err = v.Store.PublishDue(ctx, time.Now().Unix())
			}
			v.Operations.RUnlock()
			if err != nil {
				if ctx.Err() == nil {
					slog.Error("scheduled publication failed", "error", err)
				}
				break
			}
			if n < 10 {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

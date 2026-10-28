package store

import (
	"context"
	"database/sql"
	"encoding/json"
)

type AuditEvent struct {
	ID, At, UserID        int64
	Email, Action, Object string
}

func (s *Store) AuditEvents(page int) ([]AuditEvent, int, error) {
	var total int
	if err := s.DB.QueryRow("SELECT count(*) FROM audit_events").Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.Query("SELECT a.id,a.at,a.user_id,coalesce(u.email,''),a.action,a.object FROM audit_events a LEFT JOIN users u ON u.id=a.user_id ORDER BY a.id DESC LIMIT 50 OFFSET ?", (page-1)*50)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	events := []AuditEvent{}
	for rows.Next() {
		var e AuditEvent
		if err = rows.Scan(&e.ID, &e.At, &e.UserID, &e.Email, &e.Action, &e.Object); err != nil {
			return nil, 0, err
		}
		events = append(events, e)
	}
	return events, total, rows.Err()
}

func auditContent(ctx context.Context, tx *sql.Tx, action string, p Page) error {
	version := int64(0)
	if action == "schedule_failed" {
		version = p.Version
	}
	object, err := json.Marshal(struct {
		ID      int64  `json:"id"`
		Title   string `json:"title"`
		Kind    string `json:"kind"`
		Version int64  `json:"version,omitempty"`
	}{p.ID, p.Title, p.Kind, version})
	if err != nil {
		return err
	}
	return auditTx(ctx, tx, "content."+action, string(object))
}

func (s *Store) ScheduleFailed(id, version int64) (bool, error) {
	var failed bool
	err := s.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM audit_events WHERE action='content.schedule_failed' AND json_extract(object,'$.id')=? AND json_extract(object,'$.version')=?)`, id, version).Scan(&failed)
	return failed, err
}

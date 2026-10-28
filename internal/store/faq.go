package store

import (
	"context"
	"encoding/json"
)

type FAQItem struct{ Question, Answer string }
type FAQGroup struct {
	ID, Version int64
	Name        string
	Items       []FAQItem
}

func (s *Store) FAQEnabled() (bool, error) {
	var enabled bool
	err := s.DB.QueryRow("SELECT enabled FROM modules WHERE id='faq'").Scan(&enabled)
	return enabled, err
}
func (s *Store) SetFAQEnabled(ctx context.Context, enabled bool) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE modules SET enabled=? WHERE id='faq'", enabled)
	return err
}
func (s *Store) FAQGroups() ([]FAQGroup, error) {
	rows, err := s.DB.Query("SELECT id,version,name,items FROM faq_groups ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []FAQGroup{}
	for rows.Next() {
		var f FAQGroup
		var raw string
		if err = rows.Scan(&f.ID, &f.Version, &f.Name, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &f.Items); err != nil {
			return nil, err
		}
		result = append(result, f)
	}
	return result, rows.Err()
}
func (s *Store) FAQ(id int64) (FAQGroup, error) {
	var f FAQGroup
	var raw string
	err := s.DB.QueryRow("SELECT id,version,name,items FROM faq_groups WHERE id=?", id).Scan(&f.ID, &f.Version, &f.Name, &raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &f.Items)
	}
	return f, err
}
func (s *Store) SaveFAQ(ctx context.Context, f FAQGroup) (int64, error) {
	raw, err := json.Marshal(f.Items)
	if err != nil {
		return 0, err
	}
	if f.ID == 0 {
		res, err := s.DB.ExecContext(ctx, "INSERT INTO faq_groups(name,items) VALUES(?,?)", f.Name, string(raw))
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	res, err := s.DB.ExecContext(ctx, "UPDATE faq_groups SET name=?,items=?,version=version+1 WHERE id=? AND version=?", f.Name, string(raw), f.ID, f.Version)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		err = ErrConflict
	}
	return f.ID, err
}
func (s *Store) DeleteFAQ(ctx context.Context, id, version int64) error {
	res, err := s.DB.ExecContext(ctx, "DELETE FROM faq_groups WHERE id=? AND version=?", id, version)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		err = ErrConflict
	}
	return err
}

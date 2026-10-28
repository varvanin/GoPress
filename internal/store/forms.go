package store

import (
	"context"
	"encoding/json"
)

type FormField struct {
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
}
type Form struct {
	ID, Version                      int64
	Name, Recipient, Button, Success string
	Enabled                          bool
	Fields                           []FormField
}
type FormValue struct{ Label, Value string }
type FormEntry struct {
	ID, FormID, CreatedAt int64
	MailAttempts          int
	Values                []FormValue
	MailStatus            string
}

func (s *Store) Forms() ([]Form, error) {
	rows, err := s.DB.Query("SELECT id,version,name,recipient,button,success,enabled,fields FROM forms ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Form{}
	for rows.Next() {
		var f Form
		var raw string
		if err = rows.Scan(&f.ID, &f.Version, &f.Name, &f.Recipient, &f.Button, &f.Success, &f.Enabled, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &f.Fields); err != nil {
			return nil, err
		}
		result = append(result, f)
	}
	return result, rows.Err()
}
func (s *Store) Form(id int64) (Form, error) {
	var f Form
	var raw string
	err := s.DB.QueryRow("SELECT id,version,name,recipient,button,success,enabled,fields FROM forms WHERE id=?", id).Scan(&f.ID, &f.Version, &f.Name, &f.Recipient, &f.Button, &f.Success, &f.Enabled, &raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &f.Fields)
	}
	return f, err
}
func (s *Store) SaveForm(ctx context.Context, f Form) (int64, error) {
	raw, err := json.Marshal(f.Fields)
	if err != nil {
		return 0, err
	}
	if f.ID == 0 {
		res, err := s.DB.ExecContext(ctx, "INSERT INTO forms(name,recipient,button,success,enabled,fields) VALUES(?,?,?,?,?,?)", f.Name, f.Recipient, f.Button, f.Success, f.Enabled, string(raw))
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	res, err := s.DB.ExecContext(ctx, "UPDATE forms SET name=?,recipient=?,button=?,success=?,enabled=?,fields=?,version=version+1 WHERE id=? AND version=?", f.Name, f.Recipient, f.Button, f.Success, f.Enabled, string(raw), f.ID, f.Version)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		err = ErrConflict
	}
	return f.ID, err
}
func (s *Store) SaveFormEntry(ctx context.Context, f Form, values []FormValue, keys ...string) (int64, error) {
	raw, err := json.Marshal(values)
	if err != nil {
		return 0, err
	}
	var key any
	if len(keys) > 0 && keys[0] != "" {
		key = keys[0]
	}
	res, err := s.DB.ExecContext(ctx, "INSERT INTO form_entries(form_id,values_json,submission_key) SELECT id,?,? FROM forms WHERE id=? AND version=? AND enabled=1 ON CONFLICT(submission_key) WHERE submission_key IS NOT NULL DO NOTHING", string(raw), key, f.ID, f.Version)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n == 0 {
		var id int64
		if key != nil {
			err = s.DB.QueryRowContext(ctx, "SELECT id FROM form_entries WHERE submission_key=? AND form_id=?", key, f.ID).Scan(&id)
			if err == nil {
				return id, nil
			}
		}
		return 0, ErrConflict
	}
	return res.LastInsertId()
}

func (s *Store) FormEntry(id, formID int64) (FormEntry, error) {
	var e FormEntry
	var raw string
	err := s.DB.QueryRow("SELECT id,form_id,created_at,values_json,mail_status FROM form_entries WHERE id=? AND form_id=?", id, formID).Scan(&e.ID, &e.FormID, &e.CreatedAt, &raw, &e.MailStatus)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &e.Values)
	}
	return e, err
}
func (s *Store) FormEntries(id int64, page int) ([]FormEntry, int, error) {
	var total int
	if err := s.DB.QueryRow("SELECT count(*) FROM form_entries WHERE form_id=?", id).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.Query("SELECT id,form_id,created_at,values_json,mail_status FROM form_entries WHERE form_id=? ORDER BY id DESC LIMIT 25 OFFSET ?", id, (page-1)*25)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := []FormEntry{}
	for rows.Next() {
		var e FormEntry
		var raw string
		if err = rows.Scan(&e.ID, &e.FormID, &e.CreatedAt, &raw, &e.MailStatus); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal([]byte(raw), &e.Values); err != nil {
			return nil, 0, err
		}
		result = append(result, e)
	}
	return result, total, rows.Err()
}

package store

import (
	"context"
	"encoding/json"
)

type CollectionItem struct{ Image, Caption, Name, Company, Text string }
type Collection struct {
	ID, Version        int64
	Kind, Name, Layout string
	ItemCount          int
	Items              []CollectionItem
}

func (s *Store) ModuleEnabled(kind string) (bool, error) {
	var enabled bool
	err := s.DB.QueryRow("SELECT enabled FROM modules WHERE id=?", kind).Scan(&enabled)
	return enabled, err
}
func (s *Store) SetModuleEnabled(ctx context.Context, kind string, enabled bool) error {
	res, err := s.DB.ExecContext(ctx, "UPDATE modules SET enabled=? WHERE id=?", enabled, kind)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}
func (s *Store) Collections(kind string) ([]Collection, error) {
	rows, err := s.DB.Query("SELECT id,version,name,layout,json_array_length(items) FROM module_collections WHERE kind=? ORDER BY id DESC", kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Collection{}
	for rows.Next() {
		c := Collection{Kind: kind}
		var count int
		if err = rows.Scan(&c.ID, &c.Version, &c.Name, &c.Layout, &count); err != nil {
			return nil, err
		}
		c.ItemCount = count
		result = append(result, c)
	}
	return result, rows.Err()
}
func (s *Store) Collection(kind string, id int64) (Collection, error) {
	c := Collection{Kind: kind}
	var raw string
	err := s.DB.QueryRow("SELECT id,version,name,layout,items FROM module_collections WHERE id=? AND kind=?", id, kind).Scan(&c.ID, &c.Version, &c.Name, &c.Layout, &raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &c.Items)
	}
	return c, err
}
func (s *Store) SaveCollection(ctx context.Context, c Collection) (int64, error) {
	raw, err := json.Marshal(c.Items)
	if err != nil {
		return 0, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if c.ID == 0 {
		res, e := tx.ExecContext(ctx, "INSERT INTO module_collections(kind,name,layout,items) VALUES(?,?,?,?)", c.Kind, c.Name, c.Layout, string(raw))
		if e != nil {
			return 0, e
		}
		c.ID, err = res.LastInsertId()
		if err != nil {
			return 0, err
		}
	} else {
		res, e := tx.ExecContext(ctx, "UPDATE module_collections SET name=?,layout=?,items=?,version=version+1 WHERE id=? AND kind=? AND version=?", c.Name, c.Layout, string(raw), c.ID, c.Kind, c.Version)
		if e != nil {
			return 0, e
		}
		n, e := res.RowsAffected()
		if e != nil {
			return 0, e
		}
		if n != 1 {
			return 0, ErrConflict
		}
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM module_collection_media WHERE collection_id=?", c.ID); err != nil {
		return 0, err
	}
	for _, item := range c.Items {
		if item.Image != "" {
			if _, err = tx.ExecContext(ctx, "INSERT INTO module_collection_media(collection_id,media_name) VALUES(?,?) ON CONFLICT DO NOTHING", c.ID, item.Image); err != nil {
				return 0, err
			}
		}
	}
	return c.ID, tx.Commit()
}
func (s *Store) DeleteCollection(ctx context.Context, kind string, id, version int64) error {
	res, err := s.DB.ExecContext(ctx, "DELETE FROM module_collections WHERE kind=? AND id=? AND version=?", kind, id, version)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}

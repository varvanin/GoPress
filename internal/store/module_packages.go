package store

import "context"

type ModulePackage struct {
	ID                                string
	Version                           int64
	Enabled                           bool
	Manifest, Template, CSS, Settings string
	Screenshot                        []byte
	ScreenshotMIME                    string
}

func (s *Store) ModulePackages() ([]ModulePackage, error) {
	rows, err := s.DB.Query("SELECT id,version,enabled,manifest FROM module_packages ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ModulePackage{}
	for rows.Next() {
		var p ModulePackage
		if err = rows.Scan(&p.ID, &p.Version, &p.Enabled, &p.Manifest); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
func (s *Store) ModulePackage(id string) (ModulePackage, error) {
	var p ModulePackage
	err := s.DB.QueryRow("SELECT id,version,enabled,manifest,template,css,settings,screenshot,screenshot_mime FROM module_packages WHERE id=?", id).Scan(&p.ID, &p.Version, &p.Enabled, &p.Manifest, &p.Template, &p.CSS, &p.Settings, &p.Screenshot, &p.ScreenshotMIME)
	return p, err
}
func (s *Store) InstallModulePackage(ctx context.Context, p ModulePackage) error {
	_, err := s.DB.ExecContext(ctx, "INSERT INTO module_packages(id,manifest,template,css,settings,screenshot,screenshot_mime) VALUES(?,?,?,?,?,?,?)", p.ID, p.Manifest, p.Template, p.CSS, p.Settings, p.Screenshot, p.ScreenshotMIME)
	return err
}
func (s *Store) ConfigureModulePackage(ctx context.Context, id string, version int64, settings string) error {
	return s.changeModulePackage(ctx, "UPDATE module_packages SET settings=?,version=version+1 WHERE id=? AND version=?", settings, id, version)
}
func (s *Store) ToggleModulePackage(ctx context.Context, id string, version int64, enabled bool) error {
	return s.changeModulePackage(ctx, "UPDATE module_packages SET enabled=?,version=version+1 WHERE id=? AND version=?", enabled, id, version)
}
func (s *Store) DeleteModulePackage(ctx context.Context, id string, version int64) error {
	return s.changeModulePackage(ctx, "DELETE FROM module_packages WHERE id=? AND version=? AND enabled=0", id, version)
}
func (s *Store) changeModulePackage(ctx context.Context, query string, args ...any) error {
	res, err := s.DB.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		err = ErrConflict
	}
	return err
}

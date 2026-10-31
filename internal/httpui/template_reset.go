package httpui

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing/fstest"

	"github.com/labstack/echo/v5"
	"gopress/internal/service"
	"gopress/internal/store"
)

func templateTreeHash(files fstest.MapFS) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, name := range names {
		fmt.Fprintf(h, "%d:%s:%d:", len(name), name, len(files[name].Data))
		h.Write(files[name].Data)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (a *App) resetTemplateReview(c *echo.Context, status int, message string) error {
	id := c.QueryParam("theme")
	if c.Request().Method == "POST" {
		id = c.FormValue("theme")
	}
	if !builtinTemplate(id) {
		return service.ErrInvalid
	}
	a.themeMu.RLock()
	files, _, _, err := a.templateFiles(id, "", false)
	a.themeMu.RUnlock()
	if err != nil {
		return err
	}
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	return htmlComponent(c, status, TemplateReset(site.Title, a.csrfToken(c), id, templateTreeHash(files), len(files), message))
}
func (a *App) resetTemplate(id, revision string) error {
	a.themeMu.Lock()
	defer a.themeMu.Unlock()
	if !builtinTemplate(id) {
		return service.ErrInvalid
	}
	files, dir, _, err := a.templateFiles(id, "", false)
	if err != nil {
		return err
	}
	if templateTreeHash(files) != revision {
		return store.ErrConflict
	}
	if _, err = os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	parent := filepath.Dir(a.templateHistoryPath(id, "layout.html"))
	if err = os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	backup := filepath.Join(parent, "before-reset")
	rollback, err := os.MkdirTemp(parent, ".reset-")
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(rollback)
		}
	}()
	previous := filepath.Join(rollback, "previous")
	err = os.Rename(backup, previous)
	hadPrevious := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err = os.Rename(dir, backup); err != nil {
		if hadPrevious {
			if e := os.Rename(previous, backup); e != nil {
				keep = true
				return fmt.Errorf("reset failed: %w; previous reset backup retained at %s: %v", err, previous, e)
			}
		}
		return err
	}
	return nil
}

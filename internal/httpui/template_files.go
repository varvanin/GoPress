package httpui

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing/fstest"

	"github.com/labstack/echo/v5"
	"gopress/internal/service"
	"gopress/internal/store"
)

func templateFileReferenced(files fstest.MapFS, name string) bool {
	names := []string{name}
	if strings.HasSuffix(name, ".html") {
		names = []string{`"` + name + `"`, `"` + strings.TrimSuffix(name, ".html") + `"`, "`" + name + "`", "`" + strings.TrimSuffix(name, ".html") + "`"}
	}
	if strings.HasPrefix(name, "assets/") {
		names = append(names, strings.TrimPrefix(name, "assets/"), filepath.Base(name))
	}
	for other, file := range files {
		if other == name || !editableTemplateFile(other) {
			continue
		}
		for _, reference := range names {
			if bytes.Contains(file.Data, []byte(reference)) {
				return true
			}
		}
	}
	return false
}
func (a *App) changeTemplateFile(id, name, revision, content, newName, operation string) (string, error) {
	a.themeMu.Lock()
	defer a.themeMu.Unlock()
	files, dir, size, err := a.templateFiles(id, "", false)
	if err != nil {
		return name, err
	}
	old, ok := files[name]
	if !ok || !editableTemplateFile(name) || len(old.Data) > 512<<10 {
		return name, service.ErrInvalid
	}
	if templateFileHash(old.Data) != revision {
		return name, store.ErrConflict
	}
	if content != string(old.Data) {
		return name, errors.New("Сначала сохраните изменения файла.")
	}
	if operation != "delete" {
		if !editableTemplateFile(newName) || !fs.ValidPath(newName) || (!strings.Contains(newName, "/") && !service.ValidPageTemplate(newName) && newName != "home.html" && newName != "single.html" && newName != "archive.html" && newName != "search.html" && newName != "404.html" && newName != "header.html" && newName != "footer.html") {
			return name, service.ErrInvalid
		}
		if _, exists := files[newName]; exists {
			return name, errors.New("Файл с таким именем уже существует.")
		}
	}
	if operation != "create" {
		if name == "layout.html" || name == "page.html" || templateFileReferenced(files, name) {
			return name, errors.New("Файл используется шаблоном. Сначала измените ссылки на него.")
		}
		used, e := a.Service.Store.PageTemplateUsed(name)
		if e != nil {
			return name, e
		}
		if used {
			return name, errors.New("Файл используется материалом или его историей.")
		}
	}
	switch operation {
	case "create":
		data := []byte{}
		if !strings.Contains(newName, "/") {
			data = files["page.html"].Data
		}
		if len(files) >= 256 || size+len(data) > 32<<20 || len(data) > 512<<10 {
			return name, service.ErrInvalid
		}
		files[newName] = &fstest.MapFile{Data: data, Mode: 0600}
	case "rename":
		files[newName] = old
		delete(files, name)
	case "delete":
		delete(files, name)
	default:
		return name, service.ErrInvalid
	}
	if err = validateThemeFiles(files, "", store.Appearance{Theme: id}); err != nil {
		return name, err
	}
	if builtinTemplate(id) {
		if _, e := os.Stat(dir); errors.Is(e, os.ErrNotExist) {
			staged, e := stageBuiltinTemplate(dir, files)
			if e != nil {
				return name, e
			}
			defer os.RemoveAll(staged)
			if operation == "delete" {
				history := a.templateHistoryPath(id, name) + ".deleted"
				if e = os.MkdirAll(filepath.Dir(history), 0700); e != nil {
					return name, e
				}
				stagedHistory, e := stageTemplateFile(history, old.Data)
				if e != nil {
					return name, e
				}
				defer os.Remove(stagedHistory)
				if e = commitTemplateSave(staged, dir, stagedHistory, history); e != nil {
					return name, e
				}
			} else if e = os.Rename(staged, dir); e != nil {
				return name, e
			}

			if operation == "delete" {
				return "layout.html", nil
			}
			return newName, nil
		} else if e != nil {
			return name, e
		}
	}
	target := filepath.Join(dir, filepath.FromSlash(name))
	switch operation {
	case "create":
		destination := filepath.Join(dir, filepath.FromSlash(newName))
		if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return name, err
		}
		staged, e := stageTemplateFile(destination, files[newName].Data)
		if e != nil {
			return name, e
		}
		defer os.Remove(staged)
		if e = os.Link(staged, destination); e != nil {
			return name, e
		}
	case "rename":
		destination := filepath.Join(dir, filepath.FromSlash(newName))
		if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return name, err
		}
		if err = os.Link(target, destination); err != nil {
			return name, err
		}
		if err = os.Remove(target); err != nil {
			return name, errors.Join(err, os.Remove(destination))
		}
	case "delete":
		history := a.templateHistoryPath(id, name) + ".deleted"
		if err = os.MkdirAll(filepath.Dir(history), 0700); err != nil {
			return name, err
		}
		if err = os.Rename(target, history); err != nil {
			return name, err
		}
		return "layout.html", nil
	}
	return newName, nil
}
func templateZIP(files fstest.MapFS) ([]byte, error) {
	var out themeOutput
	out.limit = 8 << 20
	archive := zip.NewWriter(&out)
	names := make([]string, 0, len(files))
	for name := range files {
		if name != "seo.html" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		entry, err := archive.Create(name)
		if err != nil {
			return nil, err
		}
		data := files[name].Data
		if strings.HasSuffix(name, ".html") {
			data = templateAssetURLs(data)
		}
		if _, err = entry.Write(data); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return out.buffer.Bytes(), nil
}
func (a *App) exportTemplate(c *echo.Context) error {
	a.themeMu.RLock()
	defer a.themeMu.RUnlock()
	id := c.QueryParam("theme")
	files, _, _, err := a.templateFiles(id, "", false)
	if err != nil {
		return err
	}
	if _, exists := files["theme.json"]; !exists {
		info := ThemeInfo{ID: id + "-custom", Name: id + " custom", Version: "1.0", Author: "GoPress"}
		if id == "custom" {
			info.ID = "custom-export"
		}
		raw, _ := json.Marshal(info)
		files["theme.json"] = &fstest.MapFile{Data: raw}
		preview := "journal"
		if builtinTemplate(id) {
			preview = id
		}
		raw, err = resources.ReadFile("assets/template-" + preview + ".svg")
		if err != nil {
			return err
		}
		files["screenshot.svg"] = &fstest.MapFile{Data: raw}
	}
	raw, err := templateZIP(files)
	if err != nil {
		return echo.NewHTTPError(400, "Экспорт превышает ограничения ZIP. Проверьте размер файлов.")
	}
	c.Response().Header().Set("Content-Disposition", `attachment; filename="gopress-template.zip"`)
	return c.Blob(200, "application/zip", raw)
}

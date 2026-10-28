package httpui

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing/fstest"
	"unicode/utf8"

	"github.com/labstack/echo/v5"
	"gopress/internal/service"
	"gopress/internal/store"
)

func editableTemplateFile(name string) bool {
	if name == "" || len(name) > 240 || strings.ContainsAny(name, "\\:") || strings.HasPrefix(name, "/") || path.Clean(name) != name || strings.HasPrefix(name, "../") {
		return false
	}
	if strings.HasPrefix(name, "parts/") {
		return strings.HasSuffix(name, ".html")
	}
	if strings.HasPrefix(name, "assets/") {
		return strings.HasSuffix(name, ".css") || strings.HasSuffix(name, ".js")
	}
	return !strings.Contains(name, "/") && strings.HasSuffix(name, ".html") && name != "seo.html"
}
func templateEditorURL(id, file string) string {
	return "/admin/themes/editor?" + url.Values{"theme": {id}, "file": {file}}.Encode()
}
func templateFileHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (a *App) templateFiles(id string) (fstest.MapFS, string, error) {
	if !service.ValidTheme(id) || id == "" {
		return nil, "", service.ErrInvalid
	}
	source, prefix, resolved, err := a.themeSource(id)
	if err != nil {
		return nil, "", err
	}
	if resolved != id {
		return nil, "", service.ErrInvalid
	}
	result := fstest.MapFS{}
	root := strings.TrimSuffix(prefix, "/")
	if root == "" {
		root = "."
	}
	total := 0
	err = fs.WalkDir(source, root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(name, prefix)
		if name == root {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not editable")
		}
		if d.IsDir() {
			if rel != "parts" && rel != "assets" && !strings.HasPrefix(rel, "parts/") && !strings.HasPrefix(rel, "assets/") {
				return fs.SkipDir
			}
			return nil
		}
		stat, err := d.Info()
		if err != nil {
			return err
		}
		if !stat.Mode().IsRegular() || stat.Size() > 8<<20 {
			return service.ErrInvalid
		}
		raw, err := fs.ReadFile(source, name)
		if err != nil {
			return err
		}
		total += len(raw)
		if total > 32<<20 || len(result) >= 256 {
			return service.ErrInvalid
		}
		result[rel] = &fstest.MapFile{Data: raw, Mode: 0600}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	if prefix != "" {
		css := "public.css"
		if id == "minimal" {
			css = "public-minimal.css"
		}
		if id == "studio" {
			css = "public-studio.css"
		}
		assets := []string{css}
		if id == "studio" {
			assets = append(assets, "public-common.css")
		}
		if id == "journal" {
			assets = append(assets, "journal-cover-1.webp", "journal-cover-2.webp", "journal-cover-3.webp")
		}
		for _, name := range assets {
			raw, err := resources.ReadFile("assets/" + name)
			if err != nil {
				return nil, "", err
			}
			result["assets/"+name] = &fstest.MapFile{Data: raw, Mode: 0600}
		}
		return result, "", nil
	}
	dir := filepath.Join(a.Service.Store.Dir, "theme")
	if strings.HasPrefix(id, "theme:") {
		dir = filepath.Join(dir, "packages", strings.TrimPrefix(id, "theme:"))
	}
	return result, dir, nil
}
func (a *App) templateEditor(c *echo.Context, status int, message, submitted string, failed bool) error {
	a.themeMu.RLock()
	defer a.themeMu.RUnlock()
	id := c.QueryParam("theme")
	if c.Request().Method == "POST" {
		id = c.FormValue("theme")
	}
	files, dir, err := a.templateFiles(id)
	if err != nil {
		return err
	}
	names := []string{}
	for name := range files {
		if editableTemplateFile(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	selected := c.QueryParam("file")
	if c.Request().Method == "POST" {
		selected = c.FormValue("file")
	}
	if selected == "" {
		selected = "layout.html"
	}
	file, ok := files[selected]
	if !ok || !editableTemplateFile(selected) {
		return echo.NewHTTPError(404, "Файл не найден")
	}
	if len(file.Data) > 512<<10 {
		return echo.NewHTTPError(400, "Файл превышает 512 KiB")
	}
	content := string(file.Data)
	revision := templateFileHash(file.Data)
	if failed {
		content = submitted
		revision = c.FormValue("revision")
	}
	history := false
	if dir != "" {
		_, e := os.Stat(a.templateHistoryPath(id, selected))
		history = e == nil
	}
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	return htmlComponent(c, status, TemplateEditor(site.Title, a.csrfToken(c), id, names, selected, content, revision, message, dir == "", history, failed))
}
func (a *App) templateHistoryPath(id, name string) string {
	return filepath.Join(a.Service.Store.Dir, "theme", "editor-history", templateFileHash([]byte(id)), templateFileHash([]byte(name))+".json")
}
func atomicTemplateFile(filename string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(filename), ".gopress-edit-")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err = temp.Chmod(0600); err == nil {
		_, err = temp.Write(data)
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, filename)
}
func (a *App) saveTemplateFile(id, name, revision, content string, restore bool) error {
	a.themeMu.Lock()
	defer a.themeMu.Unlock()
	files, dir, err := a.templateFiles(id)
	if err != nil {
		return err
	}
	old, ok := files[name]
	if dir == "" || !ok || !editableTemplateFile(name) {
		return service.ErrInvalid
	}
	if templateFileHash(old.Data) != revision {
		return store.ErrConflict
	}
	historyPath := a.templateHistoryPath(id, name)
	if restore {
		raw, err := os.ReadFile(historyPath)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &content); err != nil {
			return err
		}
	}
	if !utf8.ValidString(content) || len(content) > 512<<10 {
		return service.ErrInvalid
	}
	if bytes.Equal(old.Data, []byte(content)) {
		return nil
	}
	files[name] = &fstest.MapFile{Data: []byte(content), Mode: 0600}
	if err = validateThemeFiles(files, "", store.Appearance{Theme: id}); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(historyPath), 0700); err != nil {
		return err
	}
	previous, _ := json.Marshal(string(old.Data))
	if err = atomicTemplateFile(historyPath, previous); err != nil {
		return err
	}
	return atomicTemplateFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(content))
}
func (a *App) copyBuiltinTemplate(id string) (string, error) {
	a.themeMu.Lock()
	defer a.themeMu.Unlock()
	if id != "journal" && id != "minimal" && id != "studio" {
		return "", service.ErrInvalid
	}
	files, _, err := a.templateFiles(id)
	if err != nil {
		return "", err
	}
	name := id + "-copy"
	for i := 2; ; i++ {
		_, err = os.Stat(filepath.Join(a.Service.Store.Dir, "theme", "packages", name))
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return "", err
		}
		name = fmt.Sprintf("%s-copy-%d", id, i)
	}
	info := ThemeInfo{ID: name, Name: id + " · копия", Version: "1.0", Author: "GoPress", Description: "Редактируемая копия встроенного шаблона."}
	raw, _ := json.Marshal(info)
	files["theme.json"] = &fstest.MapFile{Data: raw}
	preview, err := resources.ReadFile("assets/template-" + id + ".svg")
	if err != nil {
		return "", err
	}
	files["screenshot.svg"] = &fstest.MapFile{Data: preview}
	delete(files, "seo.html")
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for filename, file := range files {
		data := file.Data
		if strings.HasSuffix(filename, ".html") {
			for _, css := range []string{"public.css", "public-minimal.css", "public-studio.css"} {
				data = bytes.ReplaceAll(data, []byte("/assets/"+css), []byte(`{{asset .ThemeAssets "`+css+`"}}`))
			}
		}
		entry, err := w.Create(filename)
		if err != nil {
			return "", err
		}
		if _, err = entry.Write(data); err != nil {
			return "", err
		}
	}
	if err = w.Close(); err != nil {
		return "", err
	}
	installed, err := a.installTheme(bytes.NewReader(out.Bytes()))
	return installed.ID, err
}
func (a *App) templateEditorRoutes(g *echo.Group) {
	g.GET("/themes/reference", a.templateReferencePage)
	g.GET("/themes/editor", func(c *echo.Context) error {
		message := ""
		if c.QueryParam("saved") == "1" {
			message = "Файл сохранён."
		}
		if c.QueryParam("copied") == "1" {
			message = "Копия создана. Изменения появятся на сайте после её активации."
		}
		return a.templateEditor(c, 200, message, "", false)
	})
	g.POST("/themes/editor/save", func(c *echo.Context) error {
		content := c.FormValue("content")
		id, name := c.FormValue("theme"), c.FormValue("file")
		err := a.saveTemplateFile(id, name, c.FormValue("revision"), content, c.FormValue("intent") == "restore")
		if err != nil {
			status := 400
			message := err.Error()
			if errors.Is(err, store.ErrConflict) {
				status = 409
				message = "Файл изменён в другой вкладке. Скопируйте свои правки и откройте актуальную версию."
			}
			return a.templateEditor(c, status, message, content, true)
		}
		return c.Redirect(303, templateEditorURL(id, name)+"&saved=1")
	}, a.csrf)
	g.POST("/themes/editor/copy", func(c *echo.Context) error {
		id, err := a.copyBuiltinTemplate(c.FormValue("theme"))
		if err != nil {
			return err
		}
		return c.Redirect(303, templateEditorURL(id, "layout.html")+"&copied=1")
	}, a.csrf)
}

type styleNonceKey struct{}

func adminStyleNonce(ctx context.Context) string {
	nonce, _ := ctx.Value(styleNonceKey{}).(string)
	return nonce
}

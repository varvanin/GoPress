package httpui

import (
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
func (a *App) templateFiles(id, selected string, forSave bool) (fstest.MapFS, string, int, error) {
	if !service.ValidTheme(id) || id == "" {
		return nil, "", 0, service.ErrInvalid
	}
	source, prefix, resolved, err := a.themeSource(id)
	if err != nil {
		return nil, "", 0, err
	}
	if resolved != id {
		return nil, "", 0, service.ErrInvalid
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
		var raw []byte
		if selected == "" || rel == selected || forSave && strings.HasSuffix(rel, ".html") {
			if rel == selected && stat.Size() > 512<<10 {
				return echo.NewHTTPError(400, "Файл превышает 512 KiB")
			}
			raw, err = fs.ReadFile(source, name)
			if err != nil {
				return err
			}
			if prefix != "" && strings.HasSuffix(rel, ".html") {
				raw = templateAssetURLs(raw)
			}
		}
		if raw != nil {
			total += len(raw)
		} else {
			total += int(stat.Size())
		}
		if total > 32<<20 || len(result) >= 256 {
			return service.ErrInvalid
		}
		result[rel] = &fstest.MapFile{Data: raw, Mode: 0600}
		return nil
	})
	if err != nil {
		return nil, "", 0, err
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
			var raw []byte
			if selected == "" || selected == "assets/"+name {
				raw, err = resources.ReadFile("assets/" + name)
				if err != nil {
					return nil, "", 0, err
				}
			}
			info, err := fs.Stat(resources, "assets/"+name)
			if err != nil {
				return nil, "", 0, err
			}
			total += int(info.Size())
			if total > 32<<20 {
				return nil, "", 0, service.ErrInvalid
			}
			result["assets/"+name] = &fstest.MapFile{Data: raw, Mode: 0600}
		}
		return result, a.builtinTemplatePath(id), total, nil
	}
	dir := filepath.Join(a.Service.Store.Dir, "theme")
	if builtinTemplate(id) {
		dir = a.builtinTemplatePath(id)
	} else if strings.HasPrefix(id, "theme:") {
		dir = filepath.Join(dir, "packages", strings.TrimPrefix(id, "theme:"))
	}
	return result, dir, total, nil
}
func (a *App) templateEditor(c *echo.Context, status int, message, submitted string, failed bool) error {
	a.themeMu.RLock()
	defer a.themeMu.RUnlock()
	id := c.QueryParam("theme")
	if c.Request().Method == "POST" {
		id = c.FormValue("theme")
	}
	selected := c.QueryParam("file")
	if c.Request().Method == "POST" {
		selected = c.FormValue("file")
	}
	if selected == "" {
		selected = "layout.html"
	}
	files, _, _, err := a.templateFiles(id, selected, false)
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
	_, historyErr := os.Stat(a.templateHistoryPath(id, selected))
	history := historyErr == nil
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	return htmlComponent(c, status, TemplateEditor(site.Title, a.csrfToken(c), id, names, selected, content, revision, message, string(file.Data), c.FormValue("new_name"), history, failed))
}
func (a *App) templateHistoryPath(id, name string) string {
	return filepath.Join(a.Service.Store.Dir, "theme", "editor-history", templateFileHash([]byte(id)), templateFileHash([]byte(name))+".json")
}
func stageTemplateFile(filename string, data []byte) (string, error) {
	temp, err := os.CreateTemp(filepath.Dir(filename), ".gopress-edit-")
	if err != nil {
		return "", err
	}
	name := temp.Name()
	if err = temp.Chmod(0600); err == nil {
		_, err = temp.Write(data)
	}
	if err == nil {
		err = temp.Sync()
	}
	err = errors.Join(err, temp.Close())
	if err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}
func atomicTemplateFile(filename string, data []byte) error {
	staged, err := stageTemplateFile(filename, data)
	if err != nil {
		return err
	}
	defer os.Remove(staged)
	return os.Rename(staged, filename)
}

// Keep the old history recoverable until the prepared working file is committed.
func commitTemplateSave(staged, target, stagedHistory, history string) error {
	rollback, err := os.MkdirTemp(filepath.Dir(history), ".history-")
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(rollback)
		}
	}()
	previous := filepath.Join(rollback, "previous.json")
	err = os.Rename(history, previous)
	hadHistory := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err = os.Rename(stagedHistory, history); err == nil {
		err = os.Rename(staged, target)
	}
	if err == nil {
		return nil
	}
	var restoreErr error
	if hadHistory {
		restoreErr = os.Rename(previous, history)
	} else {
		restoreErr = os.Remove(history)
		if errors.Is(restoreErr, os.ErrNotExist) {
			restoreErr = nil
		}
	}
	if restoreErr != nil {
		keep = true
		return fmt.Errorf("save failed: %w; history recovery failed: %v; previous history: %s", err, restoreErr, previous)
	}
	return err
}

func (a *App) saveTemplateFile(id, name, revision, content string, restore bool) error {
	a.themeMu.Lock()
	defer a.themeMu.Unlock()
	files, dir, size, err := a.templateFiles(id, name, true)
	if err != nil {
		return err
	}
	old, ok := files[name]
	if !ok || !editableTemplateFile(name) {
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
	if !utf8.ValidString(content) || len(content) > 512<<10 || size-len(old.Data)+len(content) > 32<<20 {
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
	target := filepath.Join(dir, filepath.FromSlash(name))
	var staged string
	if builtinTemplate(id) {
		if _, statErr := os.Stat(dir); errors.Is(statErr, os.ErrNotExist) {
			// The initial local version needs all assets; later saves need only HTML and the edited file.
			full, _, _, readErr := a.templateFiles(id, "", false)
			if readErr != nil {
				return readErr
			}
			full[name] = files[name]
			staged, err = stageBuiltinTemplate(dir, full)
			target = dir
		} else if statErr != nil {
			return statErr
		}
	}
	if staged == "" && err == nil {
		staged, err = stageTemplateFile(target, []byte(content))
	}
	if err != nil {
		return err
	}
	defer os.RemoveAll(staged)
	stagedHistory, err := stageTemplateFile(historyPath, previous)
	if err != nil {
		return err
	}
	defer os.Remove(stagedHistory)
	return commitTemplateSave(staged, target, stagedHistory, historyPath)
}

func builtinTemplate(id string) bool { return id == "journal" || id == "minimal" || id == "studio" }
func (a *App) builtinTemplatePath(id string) string {
	return filepath.Join(a.Service.Store.Dir, "theme", "builtins", id)
}
func (a *App) deletedBuiltinPath(id string) string {
	return filepath.Join(a.Service.Store.Dir, "theme", "deleted-builtins", id)
}

// Publish the whole local template only after every file has been written.
func stageBuiltinTemplate(dir string, files fstest.MapFS) (string, error) {
	root := filepath.Dir(dir)
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(root, ".edit-")
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(stage)
		}
	}()
	for name, file := range files {
		target := filepath.Join(stage, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return "", err
		}
		if err = atomicTemplateFile(target, file.Data); err != nil {
			return "", err
		}
	}
	ok = true
	return stage, nil
}
func templateAssetURLs(raw []byte) []byte {
	for _, css := range []string{"public.css", "public-minimal.css", "public-studio.css"} {
		raw = bytes.ReplaceAll(raw, []byte("/assets/"+css), []byte(`{{asset .ThemeAssets "`+css+`"}}`))
	}
	return raw
}

func (a *App) copyBuiltinTemplate(id string) (string, error) {
	a.themeMu.Lock()
	defer a.themeMu.Unlock()
	if id != "journal" && id != "minimal" && id != "studio" {
		return "", service.ErrInvalid
	}
	files, _, _, err := a.templateFiles(id, "", false)
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
	rawZIP, err := templateZIP(files)
	if err != nil {
		return "", err
	}
	installed, err := a.installTheme(bytes.NewReader(rawZIP))
	return installed.ID, err
}
func (a *App) templateEditorRoutes(g *echo.Group) {
	g.GET("/themes/reference", a.templateReferencePage)
	g.GET("/themes/editor/export", a.exportTemplate)
	g.POST("/themes/editor/preview", a.previewTemplateFile, a.csrf)
	g.GET("/themes/editor/previews/:preview/*", a.previewTemplateAsset)
	g.GET("/themes/editor/previews/:preview", a.showTemplatePreview)
	g.GET("/themes/editor/reset", func(c *echo.Context) error { return a.resetTemplateReview(c, 200, "") })
	g.POST("/themes/editor/reset", func(c *echo.Context) error {
		err := a.resetTemplate(c.FormValue("theme"), c.FormValue("revision"))
		if errors.Is(err, store.ErrConflict) {
			return a.resetTemplateReview(c, 409, "Файлы изменились. Проверьте актуальную версию перед сбросом.")
		}
		if err != nil {
			return err
		}
		return c.Redirect(303, templateEditorURL(c.FormValue("theme"), "layout.html")+"&reset=1")
	}, a.csrf)
	g.GET("/themes/editor", func(c *echo.Context) error {
		message := ""
		if c.QueryParam("reset") == "1" {
			message = "Шаблон возвращён к встроенной версии."
		}
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
		var err error
		intent := c.FormValue("intent")
		if strings.HasPrefix(intent, "file-") {
			name, err = a.changeTemplateFile(id, name, c.FormValue("revision"), content, c.FormValue("new_name"), strings.TrimPrefix(intent, "file-"))
		} else {
			err = a.saveTemplateFile(id, name, c.FormValue("revision"), content, intent == "restore")
		}
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

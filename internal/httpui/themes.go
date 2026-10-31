package httpui

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v5"
	"gopress/internal/service"
	"gopress/internal/store"
)

type ThemeInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Author      string `json:"author"`
	Description string `json:"description"`
}

func legacyTheme(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "theme", "layout.html"))
	return err == nil && info.Mode().IsRegular()
}
func (a *App) themes() ([]ThemeInfo, error) {
	themes := []ThemeInfo{{ID: "journal", Name: "Журнал", Version: "1.0", Author: "GoPress", Description: "Редакционная лента, категории и удобное чтение."}, {ID: "minimal", Name: "Минимал", Version: "1.0", Author: "GoPress", Description: "Личный сайт с боковой навигацией, текстовой лентой и спокойным чтением."}, {ID: "studio", Name: "Студия", Version: "1.0", Author: "GoPress", Description: "Тёмное портфолио с крупными заголовками и сеткой проектов."}}
	visible := themes[:0]
	for _, theme := range themes {
		if _, err := os.Stat(a.deletedBuiltinPath(theme.ID)); errors.Is(err, os.ErrNotExist) {
			visible = append(visible, theme)
		} else if err != nil {
			return nil, err
		}
	}
	themes = visible
	if legacyTheme(a.Service.Store.Dir) {
		themes = append(themes, ThemeInfo{ID: "custom", Name: "Локальный шаблон", Description: "Шаблоны из папки theme."})
	}
	entries, err := os.ReadDir(filepath.Join(a.Service.Store.Dir, "theme", "packages"))
	if errors.Is(err, os.ErrNotExist) {
		return themes, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		id := "theme:" + entry.Name()
		if !entry.IsDir() || !service.ValidTheme(id) {
			continue
		}
		raw, e := os.ReadFile(filepath.Join(a.Service.Store.Dir, "theme", "packages", entry.Name(), "theme.json"))
		if e != nil {
			return nil, e
		}
		var info ThemeInfo
		if e = json.Unmarshal(raw, &info); e != nil {
			return nil, e
		}
		info.ID = id
		themes = append(themes, info)
	}
	return themes, nil
}

// Extract into a private staging directory; only a validated complete theme is published.
func (a *App) installTheme(reader io.Reader) (ThemeInfo, error) {
	var info ThemeInfo
	data, err := io.ReadAll(io.LimitReader(reader, service.MaxUpload+1))
	if err != nil {
		return info, err
	}
	if len(data) > service.MaxUpload {
		return info, fmt.Errorf("ZIP не должен превышать 8 MiB")
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return info, fmt.Errorf("Не удалось прочитать ZIP-архив")
	}
	if len(archive.File) == 0 || len(archive.File) > 256 {
		return info, fmt.Errorf("В архиве должно быть от 1 до 256 файлов и папок")
	}
	prefix := ""
	for _, file := range archive.File {
		if file.Name == "theme.json" {
			prefix = ""
			break
		}
		if strings.Count(file.Name, "/") == 1 && strings.HasSuffix(file.Name, "/theme.json") {
			prefix = strings.TrimSuffix(file.Name, "theme.json")
		}
	}
	root := filepath.Join(a.Service.Store.Dir, "theme", "packages")
	if err = os.MkdirAll(root, 0700); err != nil {
		return info, err
	}
	stage, err := os.MkdirTemp(root, ".install-")
	if err != nil {
		return info, err
	}
	defer os.RemoveAll(stage)
	seen := map[string]bool{}
	var total uint64
	for _, file := range archive.File {
		name := file.Name
		if strings.ContainsAny(name, "\\:") || strings.HasPrefix(name, "/") || strings.TrimSuffix(name, "/") != path.Clean(name) {
			return info, fmt.Errorf("Недопустимый путь в ZIP")
		}
		if prefix != "" && name == strings.TrimSuffix(prefix, "/")+"/" {
			continue
		}
		if strings.HasPrefix(name, "__MACOSX/") || path.Base(name) == ".DS_Store" {
			continue
		}
		if !strings.HasPrefix(name, prefix) {
			return info, fmt.Errorf("Все файлы должны находиться в одной папке шаблона")
		}
		name = strings.TrimPrefix(name, prefix)
		if name == "" || name == ".." || strings.HasPrefix(name, "../") || seen[name] {
			return info, fmt.Errorf("Недопустимый или повторяющийся путь в ZIP")
		}
		seen[name] = true
		if file.Mode()&os.ModeSymlink != 0 || (!file.Mode().IsRegular() && !file.FileInfo().IsDir()) {
			return info, fmt.Errorf("Ссылки и специальные файлы запрещены")
		}
		if file.FileInfo().IsDir() {
			continue
		}
		ext := strings.ToLower(path.Ext(name))
		allowed := name == "theme.json" || name == "README.md" || name == "TEMPLATE_REFERENCE.md" || name == "layout.html" || name == "header.html" || name == "footer.html" || service.ValidPageTemplate(name) || name == "home.html" || name == "single.html" || name == "archive.html" || name == "search.html" || name == "404.html" || name == "screenshot.png" || name == "screenshot.jpg" || name == "screenshot.svg"
		if strings.HasPrefix(name, "parts/") && ext == ".html" && len(name) <= 240 {
			allowed = true
		}
		if strings.HasPrefix(name, "assets/") {
			switch ext {
			case ".css", ".js", ".png", ".jpg", ".jpeg", ".webp", ".svg", ".ico", ".woff", ".woff2":
				allowed = true
			}
		}
		if !allowed {
			return info, fmt.Errorf("Неподдерживаемый файл: %s", name)
		}
		if file.UncompressedSize64 > 8<<20 || total+file.UncompressedSize64 > 32<<20 {
			return info, fmt.Errorf("Распакованный шаблон не должен превышать 32 MiB; один файл — 8 MiB")
		}
		total += file.UncompressedSize64
		input, e := file.Open()
		if e != nil {
			return info, e
		}
		content, e := io.ReadAll(io.LimitReader(input, 8<<20+1))
		closeErr := input.Close()
		if e != nil {
			return info, e
		}
		if closeErr != nil {
			return info, closeErr
		}
		if len(content) > 8<<20 {
			return info, fmt.Errorf("Файл шаблона слишком большой")
		}
		target := filepath.Join(stage, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return info, err
		}
		if err = os.WriteFile(target, content, 0600); err != nil {
			return info, err
		}
	}
	raw, err := os.ReadFile(filepath.Join(stage, "theme.json"))
	if err != nil {
		return info, fmt.Errorf("В архиве отсутствует theme.json")
	}
	if len(raw) > 16384 || json.Unmarshal(raw, &info) != nil || !service.ValidTheme("theme:"+info.ID) || strings.TrimSpace(info.Name) == "" || len(info.Name) > 120 || len(info.Version) > 40 || len(info.Author) > 120 || len(info.Description) > 600 {
		return info, fmt.Errorf("Проверьте id, name и длину полей в theme.json")
	}
	screenshot, screenshotMIME, err := themeScreenshot(os.DirFS(stage))
	if err != nil {
		return info, fmt.Errorf("Добавьте в ZIP изображение screenshot.png, screenshot.jpg или screenshot.svg")
	}
	if err = validateScreenshot(screenshot, screenshotMIME); err != nil {
		return info, err
	}

	if err = validateThemeFiles(os.DirFS(stage), "", store.Appearance{Theme: "theme:" + info.ID}); err != nil {
		return info, fmt.Errorf("Ошибка шаблона: %w", err)
	}

	target := filepath.Join(root, info.ID)
	if _, err = os.Lstat(target); err == nil {
		return info, fmt.Errorf("Шаблон с id %s уже установлена. Используйте другой id", info.ID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return info, err
	}
	if err = os.Rename(stage, target); err != nil {
		return info, err
	}
	info.ID = "theme:" + info.ID
	return info, nil
}

func (a *App) themeUploadPage(c *echo.Context, status int, message string) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	return htmlComponent(c, status, ThemeUploadPage(site.Title, a.csrfToken(c), message))
}

func (a *App) themeRoutes(g *echo.Group) {
	a.templateEditorRoutes(g)
	g.GET("/themes/upload", func(c *echo.Context) error { return a.themeUploadPage(c, 200, "") })
	g.POST("/themes/upload", func(c *echo.Context) error {
		a.themeMu.Lock()
		defer a.themeMu.Unlock()
		file, err := c.FormFile("theme_zip")
		if err != nil {
			return a.themeUploadPage(c, 400, "Выберите ZIP-архив шаблона.")
		}
		reader, err := file.Open()
		if err != nil {
			return err
		}
		defer reader.Close()
		if _, err = a.installTheme(reader); err != nil {
			return a.themeUploadPage(c, 400, err.Error())
		}
		return c.Redirect(303, "/admin/themes?installed=1")
	}, a.csrf)
	g.POST("/themes/delete", func(c *echo.Context) error {
		a.themeMu.Lock()
		defer a.themeMu.Unlock()
		id := c.FormValue("theme")
		current, err := a.Service.Store.Appearance()
		if err != nil {
			return err
		}
		_, _, active, err := a.themeSource(current.Theme)
		if err != nil {
			return err
		}
		if (!strings.HasPrefix(id, "theme:") && !builtinTemplate(id)) || !service.ValidTheme(id) || active == id {
			return a.appearancePage(c, 400, "Активный шаблон нельзя удалить.", nil)
		}
		if builtinTemplate(id) {
			marker := a.deletedBuiltinPath(id)
			if err = os.MkdirAll(filepath.Dir(marker), 0700); err != nil {
				return err
			}
			if err = atomicTemplateFile(marker, []byte("deleted\n")); err != nil {
				return err
			}
			if err = os.RemoveAll(a.builtinTemplatePath(id)); err != nil {
				return err
			}
			return c.Redirect(303, "/admin/themes?deleted=1")
		}
		// Rename first so new requests cannot select a partially deleted theme.
		root := filepath.Join(a.Service.Store.Dir, "theme", "packages")
		target := filepath.Join(root, strings.TrimPrefix(id, "theme:"))
		stage, err := os.MkdirTemp(root, ".delete-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(stage)
		if err = os.Rename(target, filepath.Join(stage, "theme")); err != nil {
			return err
		}
		return c.Redirect(303, "/admin/themes?deleted=1")
	}, a.csrf)
	g.GET("/themes/screenshot", func(c *echo.Context) error {
		id := c.QueryParam("theme")
		if !service.ValidTheme(id) {
			return echo.NewHTTPError(404, "Шаблон не найден")
		}
		var data []byte
		mime := "image/svg+xml"
		var err error
		switch id {
		case "journal", "minimal", "studio":
			data, err = resources.ReadFile("assets/template-" + id + ".svg")
		default:
			root := filepath.Join(a.Service.Store.Dir, "theme")
			if id != "custom" {
				root = filepath.Join(root, "packages", strings.TrimPrefix(id, "theme:"))
			}
			data, mime, err = themeScreenshot(os.DirFS(root))
			if errors.Is(err, os.ErrNotExist) {
				data = []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="800"><rect width="1280" height="800" fill="#f4f4f1"/><text x="640" y="380" text-anchor="middle" font-family="sans-serif" font-size="32" fill="#626c61">Превью не добавлено</text><text x="640" y="435" text-anchor="middle" font-family="sans-serif" font-size="22" fill="#626c61">Добавьте screenshot.png в папку шаблона</text></svg>`)
				mime = "image/svg+xml"
				err = nil
			}
		}
		if err != nil {
			return err
		}
		c.Response().Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'none'; style-src 'unsafe-inline'; img-src data:; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		return c.Blob(200, mime, data)
	})

	g.GET("/themes/view", func(c *echo.Context) error {
		value, err := a.Service.Store.Appearance()
		if err != nil {
			return err
		}
		value.Theme = c.QueryParam("theme")
		if !service.ValidTheme(value.Theme) {
			return echo.NewHTTPError(404, "Шаблон не найден")
		}
		if err = a.validateTheme(value); err != nil {
			return echo.NewHTTPError(404, "Шаблон не найден")
		}
		return a.render(c, store.Page{Document: emptyDocument}, false, value)
	})
	g.GET("/themes/reference.md", func(c *echo.Context) error {
		data, err := resources.ReadFile("starter/TEMPLATE_REFERENCE.md")
		if err != nil {
			return err
		}
		c.Response().Header().Set("Content-Disposition", `attachment; filename="gopress-template-reference.md"`)
		return c.Blob(200, "text/markdown; charset=utf-8", data)
	})
	g.GET("/themes/starter.zip", func(c *echo.Context) error {
		var out bytes.Buffer
		archive := zip.NewWriter(&out)
		info := ThemeInfo{ID: "my-journal", Name: "Мой журнал", Version: "1.0", Author: "Автор шаблона", Description: "Собственный шаблон на основе GoPress Journal."}
		raw, _ := json.MarshalIndent(info, "", "  ")
		preview, err := resources.ReadFile("assets/template-journal.svg")
		if err != nil {
			return err
		}
		files := map[string][]byte{"theme.json": raw, "screenshot.svg": preview}
		err = fs.WalkDir(resources, "theme", func(name string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || name == "theme/seo.html" {
				return nil
			}
			data, err := resources.ReadFile(name)
			if err != nil {
				return err
			}
			files[strings.TrimPrefix(name, "theme/")] = bytes.ReplaceAll(data, []byte("/assets/public.css"), []byte(`{{asset .ThemeAssets "public.css"}}`))
			return nil
		})
		if err != nil {
			return err
		}
		for _, name := range []string{"README.md", "TEMPLATE_REFERENCE.md"} {
			data, err := resources.ReadFile("starter/" + name)
			if err != nil {
				return err
			}
			files[name] = data
		}

		css, err := resources.ReadFile("assets/public.css")
		if err != nil {
			return err
		}
		files["assets/public.css"] = css
		for name, data := range files {
			entry, err := archive.Create(name)
			if err != nil {
				return err
			}
			if _, err = entry.Write(data); err != nil {
				return err
			}
		}
		if err = archive.Close(); err != nil {
			return err
		}
		c.Response().Header().Set("Content-Disposition", `attachment; filename="gopress-theme-starter.zip"`)
		return c.Blob(200, "application/zip", out.Bytes())
	})
}

func (a *App) themeAsset(c *echo.Context) error {
	a.themeMu.RLock()
	defer a.themeMu.RUnlock()
	id := c.Param("theme")
	if !service.ValidTheme("theme:" + id) {
		return echo.NewHTTPError(404, "Шаблон не найден")
	}
	name := c.Param("*")
	if name == "" || strings.ContainsAny(name, "\\:") || path.Clean(name) != name || strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") {
		return echo.NewHTTPError(404, "Шаблон не найден")
	}
	dir := filepath.Join(a.Service.Store.Dir, "theme", "packages", id, "assets")
	if c.Path() == "/builtin-assets/:theme/*" {
		if !builtinTemplate(id) {
			return echo.NewHTTPError(404, "Шаблон не найден")
		}
		if _, err := os.Stat(a.deletedBuiltinPath(id)); err == nil {
			return echo.NewHTTPError(404, "Шаблон не найден")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		dir = filepath.Join(a.builtinTemplatePath(id), "assets")
	}
	root := os.DirFS(dir)
	// Installed packages contain only regular files; no directory indexes or HTML are served.
	file, err := root.Open(name)
	if err != nil {
		return echo.NewHTTPError(404, "Шаблон не найден")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return echo.NewHTTPError(404, "Шаблон не найден")
	}
	if ext := path.Ext(name); ext == ".css" || ext == ".js" {
		data, err := io.ReadAll(io.LimitReader(file, 8<<20+1))
		if err != nil {
			return err
		}
		if len(data) > 8<<20 {
			return fs.ErrInvalid
		}
		c.Response().Header().Set("Cache-Control", "no-store")
		mime := "text/css; charset=utf-8"
		if ext == ".js" {
			mime = "text/javascript; charset=utf-8"
		}
		return c.Blob(200, mime, data)
	}
	if seeker, ok := file.(io.ReadSeeker); ok {
		http.ServeContent(c.Response(), c.Request(), name, info.ModTime(), seeker)
		return nil
	}
	return fs.ErrInvalid
}

func themeScreenshot(source fs.FS) ([]byte, string, error) {
	for _, name := range []string{"screenshot.png", "screenshot.jpg", "screenshot.svg"} {
		file, err := source.Open(name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		data, err := io.ReadAll(io.LimitReader(file, 8<<20+1))
		closeErr := file.Close()
		if err != nil {
			return nil, "", err
		}
		if closeErr != nil {
			return nil, "", closeErr
		}
		if len(data) > 8<<20 {
			return nil, "", fmt.Errorf("Изображение превью превышает 8 MiB")
		}
		mime := "image/png"
		if strings.HasSuffix(name, ".jpg") {
			mime = "image/jpeg"
		}
		if strings.HasSuffix(name, ".svg") {
			mime = "image/svg+xml"
		}
		return data, mime, nil
	}
	return nil, "", fs.ErrNotExist
}
func validateScreenshot(data []byte, mime string) error {
	if mime == "image/svg+xml" {
		var root struct{ XMLName xml.Name }
		if err := xml.Unmarshal(data, &root); err != nil || root.XMLName.Local != "svg" || root.XMLName.Space != "http://www.w3.org/2000/svg" {
			return fmt.Errorf("Некорректное SVG-превью")
		}
		return nil
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || ((mime == "image/png" && format != "png") || (mime == "image/jpeg" && format != "jpeg")) || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 16000000 {
		return fmt.Errorf("Некорректное изображение превью; используйте PNG/JPEG до 16 миллионов пикселей")
	}
	return nil
}

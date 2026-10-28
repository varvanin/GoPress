package httpui

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"image"
	"io"
	"io/fs"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"text/template/parse"
	"unicode/utf8"

	"github.com/labstack/echo/v5"
	"gopress/internal/service"
	"gopress/internal/store"
)

type ModuleField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Default  string `json:"default,omitempty"`
	Required bool   `json:"required,omitempty"`
}
type ModuleInfo struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Version     string        `json:"version"`
	Author      string        `json:"author"`
	Description string        `json:"description"`
	Fields      []ModuleField `json:"fields,omitempty"`
	Enabled     bool          `json:"-"`
	Revision    int64         `json:"-"`
	Builtin     bool          `json:"-"`
}

var moduleID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)
var moduleKey = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

func moduleCode(id string) string { return `[module id="` + id + `"]` }
func (a *App) moduleCatalog() ([]ModuleInfo, error) {
	enabled, err := a.Service.Store.FAQEnabled()
	if err != nil {
		return nil, err
	}
	result := []ModuleInfo{{ID: "faq", Name: "FAQ", Version: "1.0", Author: "GoPress", Description: "Вопросы и ответы", Enabled: enabled, Builtin: true}}
	packages, err := a.Service.Store.ModulePackages()
	if err != nil {
		return nil, err
	}
	for _, p := range packages {
		var info ModuleInfo
		if err = json.Unmarshal([]byte(p.Manifest), &info); err != nil {
			return nil, err
		}
		info.Enabled = p.Enabled
		info.Revision = p.Version
		result = append(result, info)
	}
	return result, nil
}
func moduleTemplate(raw string, settings map[string]string) (*template.Template, error) {
	t, err := template.New("template.html").Option("missingkey=error").Parse(raw)
	if err != nil {
		return nil, err
	}
	// Bound template work before executing an uploaded package.
	nodes := 0
	var check func(parse.Node, int, int) error
	check = func(node parse.Node, depth, loops int) error {
		if node == nil {
			return nil
		}
		nodes++
		if depth > 32 || nodes > 2048 {
			return fmt.Errorf("template.html: template is too complex")
		}
		visit := func(n parse.Node) error { return check(n, depth+1, loops) }
		switch n := node.(type) {
		case *parse.ListNode:
			if n != nil {
				for _, child := range n.Nodes {
					if err := visit(child); err != nil {
						return err
					}
				}
			}
		case *parse.FieldNode:
			if len(n.Ident) > 1 && n.Ident[0] == "Settings" {
				if _, ok := settings[n.Ident[1]]; !ok {
					return fmt.Errorf("template.html:%d: unknown setting %s", 1+strings.Count(raw[:int(n.Position())], "\n"), n.Ident[1])
				}
			}
		case *parse.VariableNode:
			if len(n.Ident) > 2 && n.Ident[0] == "$" && n.Ident[1] == "Settings" {
				if _, ok := settings[n.Ident[2]]; !ok {
					return fmt.Errorf("template.html:%d: unknown setting %s", 1+strings.Count(raw[:int(n.Position())], "\n"), n.Ident[2])
				}
			}
		case *parse.TemplateNode:
			return fmt.Errorf("template.html:%d: template calls are not supported", n.Line)
		case *parse.ActionNode:
			return visit(n.Pipe)
		case *parse.PipeNode:
			for _, cmd := range n.Cmds {
				if err := visit(cmd); err != nil {
					return err
				}
			}
		case *parse.CommandNode:
			for _, arg := range n.Args {
				if err := visit(arg); err != nil {
					return err
				}
			}
		case *parse.IdentifierNode:
			switch n.Ident {
			case "eq", "ne", "lt", "le", "gt", "ge", "and", "or", "not", "len", "index":
			default:
				return fmt.Errorf("template.html: unsupported function %s", n.Ident)
			}
		case *parse.IfNode:
			if err := visit(n.Pipe); err != nil {
				return err
			}
			if err := visit(n.List); err != nil {
				return err
			}
			return visit(n.ElseList)
		case *parse.RangeNode:
			if len(n.Pipe.Cmds) != 1 || len(n.Pipe.Cmds[0].Args) != 1 {
				return fmt.Errorf("template.html:%d: range must iterate .Settings", n.Line)
			}
			field, ok := n.Pipe.Cmds[0].Args[0].(*parse.FieldNode)
			if !ok || len(field.Ident) != 1 || field.Ident[0] != "Settings" {
				return fmt.Errorf("template.html:%d: range must iterate .Settings", n.Line)
			}
			if loops > 0 {
				return fmt.Errorf("template.html:%d: nested ranges are not supported", n.Line)
			}
			if err := visit(n.Pipe); err != nil {
				return err
			}
			if err := check(n.List, depth+1, loops+1); err != nil {
				return err
			}
			return visit(n.ElseList)
		case *parse.WithNode:
			if err := visit(n.Pipe); err != nil {
				return err
			}
			if err := visit(n.List); err != nil {
				return err
			}
			return visit(n.ElseList)
		}
		return nil
	}
	for _, part := range t.Templates() {
		if err = check(part.Tree.Root, 0, 0); err != nil {
			return nil, err
		}
	}

	return t, nil
}

type moduleOutput struct{ bytes.Buffer }

func (w *moduleOutput) Write(p []byte) (int, error) {
	if w.Len()+len(p) > 256<<10 {
		return 0, fmt.Errorf("module output exceeds 256 KiB")
	}
	return w.Buffer.Write(p)
}
func renderModule(p store.ModulePackage) (string, error) {
	var settings map[string]string
	if err := json.Unmarshal([]byte(p.Settings), &settings); err != nil {
		return "", err
	}
	t, err := moduleTemplate(p.Template, settings)
	if err != nil {
		return "", err
	}
	var out moduleOutput
	err = t.Execute(&out, struct {
		ID       string
		Settings map[string]string
	}{p.ID, settings})
	return out.String(), err
}
func moduleSettings(info ModuleInfo, values url.Values) (map[string]string, error) {
	settings := map[string]string{}
	for _, f := range info.Fields {
		v := strings.TrimSpace(values.Get(f.Key))
		if utf8.RuneCountInString(v) > 5000 || f.Required && v == "" {
			return nil, service.ErrInvalid
		}
		if v != "" && f.Type == "url" {
			u, err := url.Parse(v)
			if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
				return nil, service.ErrInvalid
			}
		}
		settings[f.Key] = v
	}
	return settings, nil
}
func (a *App) installModule(ctx context.Context, reader io.Reader) (ModuleInfo, error) {
	var info ModuleInfo
	data, err := io.ReadAll(io.LimitReader(reader, service.MaxUpload+1))
	if err != nil {
		return info, err
	}
	if len(data) > service.MaxUpload {
		return info, fmt.Errorf("ZIP exceeds 8 MiB")
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return info, fmt.Errorf("Invalid ZIP")
	}
	if len(archive.File) == 0 || len(archive.File) > 32 {
		return info, fmt.Errorf("ZIP must contain 1–32 files")
	}
	prefix := ""
	found := false
	for _, f := range archive.File {
		if f.Name == "module.json" {
			prefix = ""
			found = true
			break
		}
		if strings.Count(f.Name, "/") == 1 && strings.HasSuffix(f.Name, "/module.json") {
			prefix = strings.TrimSuffix(f.Name, "module.json")
			found = true
		}
	}
	if !found {
		return info, fmt.Errorf("module.json is missing")
	}
	files := map[string][]byte{}
	var total uint64
	for _, f := range archive.File {
		name := f.Name
		if strings.ContainsAny(name, "\\:") || strings.HasPrefix(name, "/") || strings.TrimSuffix(name, "/") != path.Clean(name) || strings.HasPrefix(name, "../") || name == ".." {
			return info, fmt.Errorf("Invalid ZIP path: %s", name)
		}
		if f.Mode()&fs.ModeSymlink != 0 || !f.FileInfo().IsDir() && !f.Mode().IsRegular() {
			return info, fmt.Errorf("Only regular files are supported")
		}
		if strings.HasPrefix(name, "__MACOSX/") || path.Base(name) == ".DS_Store" {
			continue
		}
		if !strings.HasPrefix(name, prefix) {
			return info, fmt.Errorf("All files must share one root directory")
		}
		name = strings.TrimPrefix(name, prefix)
		if f.FileInfo().IsDir() {
			continue
		}
		switch name {
		case "module.json", "template.html", "style.css", "screenshot.png", "screenshot.jpg", "README.md":
		default:
			return info, fmt.Errorf("Unsupported file: %s", name)
		}
		if _, exists := files[name]; exists {
			return info, fmt.Errorf("Duplicate file: %s", name)
		}
		total += f.UncompressedSize64
		if total > 16<<20 || f.UncompressedSize64 > 8<<20 {
			return info, fmt.Errorf("Unpacked module exceeds limit")
		}
		r, err := f.Open()
		if err != nil {
			return info, err
		}
		raw, readErr := io.ReadAll(io.LimitReader(r, 8<<20+1))
		r.Close()
		if readErr != nil {
			return info, readErr
		}
		if len(raw) > 8<<20 {
			return info, fmt.Errorf("File exceeds limit")
		}
		files[name] = raw
	}
	if len(files["module.json"]) > 32<<10 || json.Unmarshal(files["module.json"], &info) != nil {
		return info, fmt.Errorf("Invalid module.json")
	}
	if !moduleID.MatchString(info.ID) || info.ID == "faq" || strings.TrimSpace(info.Name) == "" || utf8.RuneCountInString(info.Name) > 120 || info.Version == "" || len(info.Version) > 40 || len(info.Author) > 120 || utf8.RuneCountInString(info.Description) > 600 || len(info.Fields) > 20 {
		return info, fmt.Errorf("Invalid module metadata")
	}
	keys := map[string]bool{}
	defaults := url.Values{}
	for _, f := range info.Fields {
		if !moduleKey.MatchString(f.Key) || keys[f.Key] || strings.TrimSpace(f.Label) == "" || utf8.RuneCountInString(f.Label) > 120 {
			return info, fmt.Errorf("Invalid or duplicate setting: %s", f.Key)
		}
		keys[f.Key] = true
		switch f.Type {
		case "text", "textarea", "url":
		default:
			return info, fmt.Errorf("Unsupported field type: %s", f.Type)
		}
		defaults.Set(f.Key, f.Default)
	}
	settings, err := moduleSettings(info, defaults)
	if err != nil {
		return info, fmt.Errorf("Invalid default settings")
	}
	if len(files["template.html"]) == 0 || len(files["template.html"]) > 128<<10 || len(files["style.css"]) > 128<<10 {
		return info, fmt.Errorf("template.html and style.css: limit 128 KiB")
	}
	rawSettings, _ := json.Marshal(settings)
	manifest, _ := json.Marshal(info)
	p := store.ModulePackage{ID: info.ID, Manifest: string(manifest), Template: string(files["template.html"]), CSS: string(files["style.css"]), Settings: string(rawSettings)}
	for _, name := range []string{"screenshot.png", "screenshot.jpg"} {
		if raw, ok := files[name]; ok {
			if p.Screenshot != nil {
				return info, fmt.Errorf("Use one screenshot")
			}
			cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
			if err != nil || (format != "png" && format != "jpeg") || cfg.Width > 4000 || cfg.Height > 4000 || len(raw) > 2<<20 {
				return info, fmt.Errorf("Invalid screenshot (PNG/JPEG, max 2 MiB and 4000px)")
			}
			p.Screenshot = raw
			p.ScreenshotMIME = "image/" + format
		}
	}
	if _, err = renderModule(p); err != nil {
		return info, err
	}
	if _, err = a.Service.Store.ModulePackage(info.ID); err == nil {
		return info, fmt.Errorf("Module %s already installed; remove the disabled package before installing another version", info.ID)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return info, err
	}
	return info, a.Service.Store.InstallModulePackage(ctx, p)
}

func (a *App) moduleUploadPage(c *echo.Context, status int, message string) error {
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	return htmlComponent(c, status, ModuleUpload(site.Title, a.csrfToken(c), message))
}
func (a *App) modulePackageEditor(c *echo.Context, status int, p store.ModulePackage, message string) error {
	var info ModuleInfo
	if err := json.Unmarshal([]byte(p.Manifest), &info); err != nil {
		return err
	}
	var settings map[string]string
	if err := json.Unmarshal([]byte(p.Settings), &settings); err != nil {
		return err
	}
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	return htmlComponent(c, status, ModuleEditor(site.Title, a.csrfToken(c), info, p, settings, message))
}
func postedModule(c *echo.Context) (string, int64, error) {
	id := c.FormValue("id")
	v, err := strconv.ParseInt(c.FormValue("version"), 10, 64)
	if !moduleID.MatchString(id) || v < 1 || err != nil {
		return "", 0, service.ErrInvalid
	}
	return id, v, nil
}
func (a *App) packageModuleRoutes(g *echo.Group) {
	g.GET("/modules/upload", func(c *echo.Context) error { return a.moduleUploadPage(c, 200, "") })
	g.POST("/modules/upload", func(c *echo.Context) error {
		file, err := c.FormFile("module_zip")
		if err != nil {
			return a.moduleUploadPage(c, 400, "Выберите ZIP-архив модуля.")
		}
		r, err := file.Open()
		if err != nil {
			return err
		}
		defer r.Close()
		if _, err = a.installModule(c.Request().Context(), r); err != nil {
			return a.moduleUploadPage(c, 400, err.Error())
		}
		return c.Redirect(303, "/admin/modules?installed=1")
	}, a.csrf)
	g.GET("/modules/reference.md", func(c *echo.Context) error {
		c.Response().Header().Set("Content-Disposition", `attachment; filename="MODULE_REFERENCE.md"`)
		return c.Blob(200, "text/markdown; charset=utf-8", []byte(moduleReference))
	})
	g.GET("/modules/starter.zip", func(c *echo.Context) error {
		data, err := moduleStarter()
		if err != nil {
			return err
		}
		c.Response().Header().Set("Content-Disposition", `attachment; filename="gopress-module-starter.zip"`)
		return c.Blob(200, "application/zip", data)
	})
	g.GET("/modules/package/:module/edit", func(c *echo.Context) error {
		p, err := a.Service.Store.ModulePackage(c.Param("module"))
		if err != nil {
			return err
		}
		message := ""
		if c.QueryParam("saved") == "1" {
			message = "Настройки модуля сохранены."
		}
		return a.modulePackageEditor(c, 200, p, message)
	})
	g.POST("/modules/package/save", func(c *echo.Context) error {
		id, version, err := postedModule(c)
		if err != nil {
			return err
		}
		p, err := a.Service.Store.ModulePackage(id)
		if err != nil {
			return err
		}
		if p.Version != version {
			return store.ErrConflict
		}
		var info ModuleInfo
		if err = json.Unmarshal([]byte(p.Manifest), &info); err != nil {
			return err
		}
		if err = c.Request().ParseForm(); err != nil {
			return service.ErrInvalid
		}
		values := url.Values{}
		for _, f := range info.Fields {
			values.Set(f.Key, c.FormValue("setting_"+f.Key))
		}
		settings, err := moduleSettings(info, values)
		if err != nil {
			return a.modulePackageEditor(c, 400, p, "Проверьте обязательные поля и адреса ссылок.")
		}
		raw, _ := json.Marshal(settings)
		p.Settings = string(raw)
		if _, err = renderModule(p); err != nil {
			return a.modulePackageEditor(c, 400, p, err.Error())
		}
		if err = a.Service.Store.ConfigureModulePackage(c.Request().Context(), id, version, p.Settings); err != nil {
			return err
		}
		return c.Redirect(303, "/admin/modules/package/"+id+"/edit?saved=1")
	}, a.csrf)
	g.POST("/modules/package/toggle", func(c *echo.Context) error {
		id, version, err := postedModule(c)
		if err != nil {
			return err
		}
		value := c.FormValue("enabled")
		if value != "true" && value != "false" {
			return service.ErrInvalid
		}
		p, err := a.Service.Store.ModulePackage(id)
		if err != nil {
			return err
		}
		if value == "true" {
			if _, err = renderModule(p); err != nil {
				return a.modulePackageEditor(c, 400, p, err.Error())
			}
		}
		if err = a.Service.Store.ToggleModulePackage(c.Request().Context(), id, version, value == "true"); err != nil {
			return err
		}
		return c.Redirect(303, "/admin/modules")
	}, a.csrf)
	g.POST("/modules/package/delete", func(c *echo.Context) error {
		id, version, err := postedModule(c)
		if err != nil {
			return err
		}
		if err = a.Service.Store.DeleteModulePackage(c.Request().Context(), id, version); err != nil {
			return err
		}
		return c.Redirect(303, "/admin/modules")
	}, a.csrf)
	a.Echo.GET("/module-assets/:module/:asset", func(c *echo.Context) error {
		p, err := a.Service.Store.ModulePackage(c.Param("module"))
		if err != nil {
			return err
		}
		c.Response().Header().Set("Cache-Control", "no-cache")
		switch c.Param("asset") {
		case "style.css":
			if !p.Enabled {
				return echo.NewHTTPError(404, "Not found")
			}
			return c.Blob(200, "text/css; charset=utf-8", []byte(p.CSS))
		case "screenshot":
			if len(p.Screenshot) > 0 {
				return c.Blob(200, p.ScreenshotMIME, p.Screenshot)
			}
			return c.Blob(200, "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="800" height="420" viewBox="0 0 800 420"><rect width="800" height="420" fill="#18181b"/><rect x="240" y="100" width="320" height="220" rx="12" fill="#202024" stroke="#36363c"/><path d="M280 150h180m-180 35h240m-240 35h200m-200 45h100" stroke="#777780" stroke-width="10" stroke-linecap="round"/></svg>`))
		}
		return echo.NewHTTPError(404, "Not found")
	})
}

var moduleShortcode = regexp.MustCompile(`<p(?: [^>]*)?>\[module id=(?:"|&quot;|&#34;)([a-z][a-z0-9-]{0,47})(?:"|&quot;|&#34;)\]</p>`)

func (a *App) renderModules(c *echo.Context, body string) (string, error) {
	var renderErr error
	body = moduleShortcode.ReplaceAllStringFunc(body, func(code string) string {
		if renderErr != nil {
			return ""
		}
		id := moduleShortcode.FindStringSubmatch(code)[1]
		p, err := a.Service.Store.ModulePackage(id)
		if errors.Is(err, sql.ErrNoRows) || err == nil && !p.Enabled {
			return ""
		}
		if err != nil {
			renderErr = err
			return ""
		}
		rendered, err := renderModule(p)
		if err != nil {
			renderErr = fmt.Errorf("module %s: %w", id, err)
			return ""
		}
		return `<link rel="stylesheet" href="/module-assets/` + id + `/style.css"><section class="gp-module gp-module-` + id + `">` + rendered + `</section>`
	})
	return body, renderErr
}

//go:embed module-starter/*
var moduleStarterFiles embed.FS

//go:embed module-starter/README.md
var moduleReference string

func moduleStarter() ([]byte, error) {
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	entries, err := moduleStarterFiles.ReadDir("module-starter")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		raw, err := moduleStarterFiles.ReadFile("module-starter/" + entry.Name())
		if err != nil {
			return nil, err
		}
		file, err := w.Create(entry.Name())
		if err != nil {
			return nil, err
		}
		if _, err = file.Write(raw); err != nil {
			return nil, err
		}
	}
	if err = w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

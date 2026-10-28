package httpui

import (
	"fmt"
	"html"
	"strings"

	"github.com/labstack/echo/v5"
)

type TemplateReferenceSection struct{ ID, Title, HTML string }

func referenceInline(text string) string {
	pieces := strings.Split(text, "`")
	for i := range pieces {
		pieces[i] = html.EscapeString(pieces[i])
		if i%2 == 1 {
			pieces[i] = "<code>" + pieces[i] + "</code>"
		}
	}
	return strings.Join(pieces, "")
}

// The bundled reference uses headings, paragraphs, fenced code and tables only.
func templateReferenceSections(raw string) []TemplateReferenceSection {
	result := []TemplateReferenceSection{}
	lines := strings.Split(raw, "\n")
	var body strings.Builder
	title := "About this reference"
	flush := func() {
		if body.Len() > 0 {
			result = append(result, TemplateReferenceSection{ID: fmt.Sprintf("reference-%d", len(result)+1), Title: title, HTML: body.String()})
			body.Reset()
		}
	}
	for i := 0; i < len(lines); {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "# ") {
			i++
			continue
		}
		if strings.HasPrefix(line, "## ") {
			flush()
			title = strings.TrimPrefix(line, "## ")
			i++
			continue
		}
		if strings.HasPrefix(line, "```") {
			i++
			var code strings.Builder
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				code.WriteString(lines[i])
				code.WriteByte('\n')
				i++
			}
			body.WriteString("<pre><code>" + html.EscapeString(code.String()) + "</code></pre>")
			if i < len(lines) {
				i++
			}
			continue
		}
		if strings.HasPrefix(line, "|") {
			body.WriteString(`<div class="reference-table"><table>`)
			row := 0
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
				value := strings.TrimSpace(lines[i])
				i++
				if row == 1 && strings.Contains(value, "---") {
					row++
					continue
				}
				tag := "td"
				if row == 0 {
					tag = "th"
				}
				body.WriteString("<tr>")
				for _, cell := range strings.Split(strings.Trim(value, "|"), "|") {
					body.WriteString("<" + tag + ">" + referenceInline(strings.TrimSpace(cell)) + "</" + tag + ">")
				}
				body.WriteString("</tr>")
				row++
			}
			body.WriteString("</table></div>")
			continue
		}
		var paragraph []string
		for i < len(lines) {
			value := strings.TrimSpace(lines[i])
			if value == "" || strings.HasPrefix(value, "## ") || strings.HasPrefix(value, "```") || strings.HasPrefix(value, "|") {
				break
			}
			paragraph = append(paragraph, value)
			i++
		}
		body.WriteString("<p>" + referenceInline(strings.Join(paragraph, " ")) + "</p>")
	}
	flush()
	return result
}
func (a *App) templateReferencePage(c *echo.Context) error {
	raw, err := resources.ReadFile("starter/TEMPLATE_REFERENCE.md")
	if err != nil {
		return err
	}
	site, err := a.Service.Store.Site()
	if err != nil {
		return err
	}
	return htmlComponent(c, 200, TemplateReference(site.Title, a.csrfToken(c), templateReferenceSections(string(raw))))
}

package service

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Node is the shared, strictly validated Tiptap document format.
type Node struct {
	Type    string          `json:"type"`
	Text    string          `json:"text,omitempty"`
	Content []Node          `json:"content,omitempty"`
	Marks   []Mark          `json:"marks,omitempty"`
	Attrs   json.RawMessage `json:"attrs,omitempty"`
}
type Mark struct {
	Type  string          `json:"type"`
	Attrs json.RawMessage `json:"attrs,omitempty"`
}

var imagePath = regexp.MustCompile(`^/uploads/([a-f0-9]{64}\.(?:png|jpg))$`)
var markTags = map[string]string{"bold": "strong", "italic": "em", "code": "code", "strike": "s", "underline": "u", "highlight": "mark", "superscript": "sup", "subscript": "sub"}

var codeLanguage = regexp.MustCompile(`^[a-zA-Z0-9_+-]{1,32}$`)

// SafeLink allows public web/mail links and local paths, never script/data URLs.
func SafeLink(raw string) bool {
	if raw == "" || len(raw) > 2048 || strings.Contains(raw, "\\") || strings.IndexFunc(raw, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "http", "https":
		return u.Hostname() != "" && u.User == nil
	case "mailto":
		_, err := mail.ParseAddress(u.Opaque)
		return err == nil && u.RawQuery == "" && u.Fragment == ""
	case "":
		return strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "#")
	}
	return false
}
func decodeStrict(raw string, value any) error {
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return ErrInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return ErrInvalid
	}
	return nil
}
func noAttrs(attrs json.RawMessage) bool { return len(attrs) == 0 || string(attrs) == "null" }
func readAttrs(attrs json.RawMessage, value any) error {
	if noAttrs(attrs) {
		return nil
	}
	return decodeStrict(string(attrs), value)
}
func Render(raw string) (string, error) { return render(raw, nil) }

func render(raw string, checkMedia func(string) error) (string, error) {
	if len(raw) > MaxDocumentSize {
		return "", fmt.Errorf("%w: document too large", ErrInvalid)
	}
	var doc Node
	if err := decodeStrict(raw, &doc); err != nil {
		return "", err
	}
	if doc.Type != "doc" || doc.Text != "" || len(doc.Marks) > 0 || !noAttrs(doc.Attrs) || len(doc.Content) == 0 {
		return "", ErrInvalid
	}
	var out strings.Builder
	nodes := 0
	var write func(Node, string, int) error
	write = func(n Node, parent string, depth int) error {
		nodes++
		if depth > 32 || nodes > 10000 {
			return ErrInvalid
		}
		inline := parent == "paragraph" || parent == "heading"
		if n.Type == "text" {
			if (!inline && parent != "codeBlock") || n.Text == "" || len(n.Content) > 0 || !noAttrs(n.Attrs) || len(n.Marks) > 8 {
				return ErrInvalid
			}
			if parent == "codeBlock" && len(n.Marks) > 0 {
				return ErrInvalid
			}
			value := html.EscapeString(n.Text)
			seen := map[string]bool{}
			for _, m := range n.Marks {
				if seen[m.Type] {
					return ErrInvalid
				}
				seen[m.Type] = true
				switch m.Type {
				case "bold", "italic", "code", "strike", "underline", "highlight", "superscript", "subscript":
					if !noAttrs(m.Attrs) || m.Type == "code" && len(n.Marks) != 1 {
						return ErrInvalid
					}
					tag := markTags[m.Type]
					value = "<" + tag + ">" + value + "</" + tag + ">"
				case "link":
					var a struct{ Href, Target, Rel, Class, Title string }
					if readAttrs(m.Attrs, &a) != nil || !SafeLink(a.Href) || a.Target != "" && a.Target != "_blank" || a.Class != "" || len(a.Title) > 2000 {
						return ErrInvalid
					}
					attrs := ` href="` + html.EscapeString(a.Href) + `" rel="noopener noreferrer"`
					if a.Target == "_blank" {
						attrs += ` target="_blank"`
					}
					if a.Title != "" {
						attrs += ` title="` + html.EscapeString(a.Title) + `"`
					}
					value = "<a" + attrs + ">" + value + "</a>"
				default:
					return ErrInvalid
				}
			}
			if seen["superscript"] && seen["subscript"] {
				return ErrInvalid
			}
			out.WriteString(value)
			return nil
		}
		if n.Text != "" || len(n.Marks) > 0 {
			return ErrInvalid
		}
		if n.Type == "hardBreak" {
			if !inline || len(n.Content) > 0 || !noAttrs(n.Attrs) {
				return ErrInvalid
			}
			out.WriteString("<br>")
			return nil
		}
		if inline || parent == "codeBlock" {
			return ErrInvalid
		}
		if parent == "bulletList" || parent == "orderedList" {
			if n.Type != "listItem" {
				return ErrInvalid
			}
		} else if n.Type == "listItem" {
			return ErrInvalid
		}
		tag, attrs := "", ""
		switch n.Type {
		case "paragraph":
			tag = "p"
			var a struct{ TextAlign string }
			if readAttrs(n.Attrs, &a) != nil {
				return ErrInvalid
			}
			var err error
			attrs, err = alignmentClass(a.TextAlign)
			if err != nil {
				return err
			}
		case "heading":
			var a struct {
				Level     int
				TextAlign string
			}
			if readAttrs(n.Attrs, &a) != nil || a.Level < 1 || a.Level > 3 {
				return ErrInvalid
			}
			tag = "h" + strconv.Itoa(a.Level)
			var err error
			attrs, err = alignmentClass(a.TextAlign)
			if err != nil {
				return err
			}
		case "bulletList":
			tag = "ul"
			if !noAttrs(n.Attrs) || len(n.Content) == 0 {
				return ErrInvalid
			}
		case "orderedList":
			var a struct {
				Start *int
				Type  *string
			}
			if readAttrs(n.Attrs, &a) != nil || len(n.Content) == 0 {
				return ErrInvalid
			}
			tag = "ol"
			if a.Start != nil {
				if *a.Start < 1 || *a.Start > 1000000 {
					return ErrInvalid
				}
				attrs = ` start="` + strconv.Itoa(*a.Start) + `"`
			}
			if a.Type != nil {
				if len(*a.Type) != 1 || !strings.Contains("1aAiI", *a.Type) {
					return ErrInvalid
				}
				attrs += ` type="` + *a.Type + `"`
			}
		case "listItem":
			tag = "li"
			if !noAttrs(n.Attrs) || len(n.Content) == 0 || n.Content[0].Type != "paragraph" {
				return ErrInvalid
			}
		case "blockquote":
			tag = "blockquote"
			if !noAttrs(n.Attrs) || len(n.Content) == 0 {
				return ErrInvalid
			}
		case "codeBlock":
			var a struct{ Language string }
			if readAttrs(n.Attrs, &a) != nil {
				return ErrInvalid
			}
			tag = "pre"
			attrs = ""
			out.WriteString("<pre><code")
			if a.Language != "" {
				if !codeLanguage.MatchString(a.Language) {
					return ErrInvalid
				}
				out.WriteString(` class="language-` + a.Language + `"`)
			}
			out.WriteString(">")
			for _, child := range n.Content {
				if err := write(child, n.Type, depth+1); err != nil {
					return err
				}
			}
			out.WriteString("</code></pre>")
			return nil
		case "horizontalRule":
			if !noAttrs(n.Attrs) || len(n.Content) > 0 {
				return ErrInvalid
			}
			out.WriteString("<hr>")
			return nil
		case "image":
			var a struct{ Src, Alt, Caption string }
			if readAttrs(n.Attrs, &a) != nil || len(n.Content) > 0 || len(a.Alt) > 2000 || len(a.Caption) > 2000 {
				return ErrInvalid
			}
			match := imagePath.FindStringSubmatch(a.Src)
			if match == nil {
				return ErrInvalid
			}
			if checkMedia != nil {
				if err := checkMedia(match[1]); err != nil {
					return err
				}
			}
			out.WriteString(`<figure><img src="` + html.EscapeString(a.Src) + `" alt="` + html.EscapeString(a.Alt) + `" loading="lazy">`)
			if a.Caption != "" {
				out.WriteString("<figcaption>" + html.EscapeString(a.Caption) + "</figcaption>")
			}
			out.WriteString("</figure>")
			return nil
		default:
			return ErrInvalid
		}
		out.WriteString("<" + tag + attrs + ">")
		for _, child := range n.Content {
			if err := write(child, n.Type, depth+1); err != nil {
				return err
			}
		}
		out.WriteString("</" + tag + ">")
		return nil
	}
	for _, n := range doc.Content {
		if err := write(n, "doc", 1); err != nil {
			return "", err
		}
	}
	return out.String(), nil
}

func alignmentClass(align string) (string, error) {
	if align == "" {
		return "", nil
	}
	switch align {
	case "left", "center", "right", "justify":
		return ` class="align-` + align + `"`, nil
	}
	return "", ErrInvalid
}

package httpui

import (
	"io"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"gopress/internal/store"
)

func uploadImageName(token html.Token) string {
	if token.Data != "img" {
		return ""
	}
	for _, attr := range token.Attr {
		if attr.Key == "src" && strings.HasPrefix(attr.Val, "/uploads/") {
			name := strings.TrimPrefix(attr.Val, "/uploads/")
			if !strings.ContainsAny(name, "/?#\\") {
				return name
			}
		}
	}
	return ""
}
func (a *App) responsiveImages(raw string) (string, error) {
	if !strings.Contains(raw, "/uploads/") {
		return raw, nil
	}
	names := map[string]bool{}
	z := html.NewTokenizer(strings.NewReader(raw))
	for {
		kind := z.Next()
		if kind == html.ErrorToken {
			if z.Err() != io.EOF {
				return "", z.Err()
			}
			break
		}
		if kind == html.StartTagToken || kind == html.SelfClosingTagToken {
			if name := uploadImageName(z.Token()); name != "" {
				names[name] = true
			}
		}
	}
	if len(names) == 0 {
		return raw, nil
	}
	list := make([]string, 0, len(names))
	for name := range names {
		list = append(list, name)
	}
	images, err := a.Service.Store.ImageInfos(list)
	if err != nil {
		return "", err
	}
	if len(images) == 0 {
		return raw, nil
	}
	z = html.NewTokenizer(strings.NewReader(raw))
	var out themeOutput
	for {
		kind := z.Next()
		if kind == html.ErrorToken {
			if z.Err() != io.EOF {
				return "", z.Err()
			}
			break
		}
		data := string(z.Raw())
		if kind == html.StartTagToken || kind == html.SelfClosingTagToken {
			token := z.Token()
			name := uploadImageName(token)
			if info, ok := images[name]; ok {
				attrs := map[string]bool{}
				for _, attr := range token.Attr {
					attrs[attr.Key] = true
				}
				add := func(key, value string) {
					if !attrs[key] {
						token.Attr = append(token.Attr, html.Attribute{Key: key, Val: value})
						attrs[key] = true
					}
				}
				add("width", strconv.Itoa(info.Width))
				add("height", strconv.Itoa(info.Height))
				add("decoding", "async")
				if len(info.Variants) > 0 && !attrs["srcset"] {
					sources := map[int]string{info.Width: "/uploads/" + name}
					for _, v := range info.Variants {
						if v.Width != info.Width {
							sources[v.Width] = "/uploads/" + name + "?w=" + strconv.Itoa(v.Size)
						}
					}
					widths := make([]int, 0, len(sources))
					for width := range sources {
						widths = append(widths, width)
					}
					sort.Ints(widths)
					parts := make([]string, 0, len(widths))
					for _, width := range widths {
						parts = append(parts, sources[width]+" "+strconv.Itoa(width)+"w")
					}
					add("srcset", strings.Join(parts, ", "))
					add("sizes", "100vw")
				}
				data = token.String()
			}
		}
		if _, err = out.Write([]byte(data)); err != nil {
			return "", err
		}
	}
	return out.String(), nil
}
func imageVariantFile(name string, info store.ImageInfo, size int) string {
	for _, variant := range info.Variants {
		if variant.Size == size {
			return store.ImageVariantName(name, size)
		}
	}
	return name
}

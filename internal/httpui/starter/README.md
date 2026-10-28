# Journal — the GoPress starter template

1. Change `id`, `name` and `author` in `theme.json`. Use lowercase ASCII letters, digits and hyphens in the ID.
2. Edit HTML and `assets/public.css`. Template authors do not need PHP, a bundler or Go.
3. Replace `screenshot.svg` with your own preview image: PNG/JPG/SVG.
4. ZIP the folder contents, upload under Templates, preview and activate.

Required files: `theme.json`, a screenshot, `layout.html` and `page.html`. Other page templates are optional.

- `layout.html`: outer layout, SEO, header, content and footer.
- `header.html` / `footer.html`: shared parts, called with `{{template "header" .}}` and `{{template "footer" .}}`.
- `home.html`: the first feed page; `single.html`: an individual post.
- `archive.html`: a category or subsequent feed page.
- `search.html`: search; `404.html`: not found.
- `parts/*.html`: reusable blocks, such as `{{template "parts/post-card" .}}`.
- `assets/`: styles, images, fonts and separate JavaScript files.

Files are ordinary HTML with double-brace expressions. Values are escaped automatically. Legacy `{{define "layout"}}` / `{{define "body"}}` blocks remain supported; do not mix both conventions in one file.

See `TEMPLATE_REFERENCE.md` for data, conditions, loops, helpers and selection rules. Do not add inline JS/CSS: the public security policy requires separate same-origin asset files. Templates do not receive the database, user sessions, form submissions or SMTP passwords.

The starter shares Journal's editorial design: a leading story, two supporting stories and a feed without duplicates. Archive, search and article layouts are separate. `parts/filters.html` handles categories; `parts/icons.html` contains small SVG icons. `journalCover` supplies photographic fallback covers, with user-uploaded covers taking priority. Wrap subscription links in `{{if .SEO.RSS}}` to honor the site's RSS setting.

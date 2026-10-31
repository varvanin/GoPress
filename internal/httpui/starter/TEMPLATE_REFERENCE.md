# GoPress template reference

Templates use Go `html/template`. Write HTML/CSS with the expressions below; PHP and server-code access are not available. Dynamic values are escaped for their HTML context.

## File selection

| Request | Selection order |
| --- | --- |
| Feed home | home.html → page.html |
| Home using a selected page | Selected page template → page.html |
| Page | Selected custom *.html → page.html |
| Post | single.html → page.html |
| Category `/?term=ID` or `/?page=N` | archive.html → home.html → page.html |
| Search `/search?q=...` | search.html → page.html |
| Not found | 404.html → page.html, retaining HTTP 404 |

`layout.html` and `page.html` are required. An existing file's error does not trigger fallback: fix the installation/activation error. `home.html` represents the feed; there is no separate `/archive` route. Category and feed pagination parameters remain on `/`.

## Shared parts

In `layout.html`:

```html
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  {{template "seo" .}}
  <link rel="stylesheet" href="{{asset .ThemeAssets "public.css"}}">
</head>
<body>
  {{template "header" .}}
  <main>{{template "body" .}}</main>
  {{template "footer" .}}
</body>
</html>
```

`header.html` / `footer.html` are available as `header` / `footer`. `parts/post-card.html` is available as `parts/post-card`; nested directories work too. Pass the dot: `{{template "parts/post-card" .}}`. Filenames are case-sensitive; prefer lowercase ASCII names.

All parts load before the selected page. Other page bodies are not loaded together, so each may independently define `body`. Named blocks such as `{{define "menu"}}...{{end}}` are supported. Do not redefine the reserved `layout`, `body` or `seo` names in shared parts. The engine provides `seo`; do not include a competing implementation in a package.

## Page variables

| Variable | Meaning |
| --- | --- |
| `.Title`, `.Slug`, `.Kind` | Title, path without the initial slash and `page`/`post` type; service pages may have an empty Kind |
| `.Body` | Validated editor HTML; fallback search templates receive result HTML |
| `.Cover`, `.Description` | Stored cover name and content SEO description; use a cover helper for its URL |
| `.PublishedAt` | Unix publication timestamp |
| `.ReadingMinutes`, `.Year` | Post reading estimate and current year |
| `.Home`, `.HomeIsPage`, `.Archive`, `.NotFound`, `.Preview` | Home, selected-page home, archive, 404 and private-preview flags |
| `.Template` | Actual selected filename after fallback |
| `.Site.Title`, `.Site.Description`, `.Site.BaseURL`, `.Site.Timezone` | Site name, description, configured origin and timezone |
| `.ThemeAssets` | Current asset base: `/theme-assets/ID` for packages, `/builtin-assets/ID` for edited built-ins, `/assets` for untouched built-ins and legacy local templates |
| `.Appearance.HomeTitle`, `.Appearance.HomeDescription`, `.Appearance.Theme` | Feed heading/description and selected template identifier |
| `.SEO.RSS` | Whether RSS links should be visible; wrap custom subscription links in `{{if .SEO.RSS}}` |
| `.Language` | Effective public interface language, independent of the admin language |
| `.Languages` | Available material versions with Code, Label, URL and Current; use the shared `language-switcher` template |
| `.URL` | Current internal material URL including the selected language |
| `.SEO.Alternates` | Indexable reciprocal language links, rendered by the shared SEO block |
| `.SEO.ContentLanguage` | Selected content language code for `<html lang>`; originals use the language saved with the material |
| `.SEO` | Prepared metadata for `{{template "seo" .}}`; custom manual output is usually unnecessary |
| `.Pages` | Published page metadata for automatic navigation, in configured order |
| `.Posts` | Current page of published posts: Title, Slug, Cover, PublishedAt, SEO and TermIDs |
| `.Terms`, `.Categories`, `.PostTerms` | Site terms, categories and current-content terms: ID, Name, Kind (`category`/`tag`) |
| `.ActiveTerm` | Selected category or nil; use `with` |
| `.HeaderMenu`, `.FooterMenu` | Menu trees with Label, URL and Children |
| `.HeaderMenuConfigured` | Header menu was explicitly saved; otherwise Pages can provide automatic navigation |
| `.BlogPage`, `.BlogTotal`, `.BlogPageSize`, `.BlogTerm` | Feed page, total posts, page size and selected term ID |
| `.Pagination.Current`, `.Pagination.TotalPages`, `.Pagination.Total` | Current feed/search pagination |
| `.Pagination.PreviousURL`, `.Pagination.NextURL` | Ready internal URLs, empty at boundaries, retaining category/query |
| `.Search` | Search only: Query, Results, Total, Page; results have Title, Slug and plain-text Snippet |
| `.Featured`, `.Spotlight`, `.Feed`, `.Related` | Editorial PostView blocks, containing post metadata and an optional Category |

Posts/Pages contain published metadata, not editor documents. On Journal's first page, Featured/Spotlight/Feed partition posts without repetition; an ordinary feed uses Feed for all current posts. Simple templates can use Posts directly.

## Conditions, loops and scope

```html
{{if .Home}}
  <h1>{{.Appearance.HomeTitle}}</h1>
{{else}}
  <h1>{{.Title}}</h1>
{{end}}

{{with .ActiveTerm}}<p>{{.Name}}</p>{{end}}

{{$site := .Site}}
{{range .Posts}}
  <article>
    <h2><a href="/{{.Slug}}">{{.Title}}</a></h2>
    <time datetime="{{isoDate .PublishedAt}}">{{date .PublishedAt}}</time>
    <p>{{$site.Title}}</p>
  </article>
{{else}}
  <p>No published posts yet.</p>
{{end}}
```

`range` changes the dot to the current item. Keep root access with `$`, or capture a value such as `{{$site := .Site}}` before looping. `with` runs for a nonempty value and also changes the dot. Zero, false, an empty string/list and nil are false. Built-ins include `eq`, `ne`, `lt`, `le`, `gt`, `ge`, `and`, `or`, `not`, `len`, `index` and `printf`.

## Functions

| Expression | Result |
| --- | --- |
| `{{tr "Поиск"}}` | Known UI label in the selected public language; unknown text is retained |
| `{{siteURL "/search"}}` | Internal navigation URL retaining the effective public language |
| `{{contentURL .Slug .URL}}` | Localized content URL, with a slug fallback for older fixtures |
| `{{date .PublishedAt}}` | DD.MM.YYYY in the site's timezone |
| `{{isoDate .PublishedAt}}` | RFC3339 UTC timestamp for a time element |
| `{{asset .ThemeAssets "images/logo.svg"}}` | Current-template asset URL; absolute paths and `..` are rejected |
| `{{cover .Slug .Cover}}` | Uploaded cover or the existing slug-based illustration fallback |
| `{{journalCover .Slug .Cover}}` | Uploaded cover or a built-in editorial photograph |
| `{{firstLetter .Site.Title}}` | First character of the site name |
| `{{postCount .BlogTotal}}` | Localized post count (Russian retains grammatical endings) |
| `{{add .BlogPage 1}}` | Integer addition |
| `{{hasNext .BlogPage .BlogTotal}}` | Whether the feed has a next page, respecting configured page size |

Use `href="{{contentURL .Slug .URL}}"` for page/post list links; `.Slug` remains the original route without a query. `.Pages` and `.Posts` use an available translation’s title/SEO and `.URL`, falling back to the original when absent. Use `{{siteURL "/"}}` for navigation preserving the public language. CSS may reference neighboring assets with relative `url(...)` paths.

## Validation and security

Execution stops before exceeding 2 MiB per validation fixture or 8 MiB per public response. Each execution also has a 100,000-step budget for template calls, branches and loop iterations, including loops that produce no output. Keep nested loops and recursive parts small.

Installation/activation checks all page templates, custom pages, shared-part syntax and rendering with empty/populated fixtures. Expression errors identify a file and line. Unknown fields, missing referenced parts and invalid asset paths prevent activation. Preview conditional combinations that depend on real site values: fixtures cannot cover every branch.

Only Body is trusted server-rendered HTML. There is no raw/safeHTML helper for user text. No database, server files, sessions, passwords or submissions are exposed. Template authors are nevertheless trusted: static HTML/JavaScript can affect public visitors.

The public CSP permits local scripts/styles/fonts, not inline JS/CSS or external CDNs. ZIP limits are 8 MiB compressed, 256 entries, 32 MiB unpacked and 8 MiB per file. PHP, symlinks and traversal are rejected. The admin supplies a ready starter ZIP.

## Template file editor

Open Templates → Editor on a card. The left column lists root HTML, `parts` and CSS/JS under `assets`; the code panel provides highlighting, line numbers, search, wrapping and indentation. Ctrl/Cmd+S saves. Tab indents; Escape followed by Tab lets keyboard users leave the editor. A file must be at most 512 KiB to edit.

Built-in templates can be edited directly. The first successful save stores a complete local version under the same template ID; subsequent updates preserve it. Uploaded packages and the legacy local template also edit directly. Inactive built-in and uploaded templates can be deleted from the catalog; activate another template before deleting the current one. Deleted built-ins stay hidden after updates.

Use Format in the file header to format HTML, CSS or JavaScript with two-space indentation. Formatting runs locally in the browser, preserves Go actions and does not save the file. Undo reverts the formatting as one change; save explicitly to apply it to the site.

Before replacement, the server renders the HTML template set. A failed check leaves the live file untouched and retains edits on screen. A stale file hash raises a conflict: copy your edits and reopen the current version rather than overwriting another tab.

Restore previous version revalidates and restores the last saved content of that file. Only one previous version is kept; this is not a substitute for a full backup. History is included in normal backups. Edits to an active template are immediately public; use an inactive copy and preview for experiments. CSS/JS highlighting is not server-side syntax validation or browser verification.

Content headings and paragraphs may carry editor-defined anchors, rendered as `id="section-NAME"`. Reserve the `section-` ID prefix for these content targets. Navigation links such as `/#section-contacts` work for one-page homepages. Preserve IDs in `.Body`; add scroll margin if your header is fixed.

## File operations and private preview

File operations require saved source and retain optimistic file hashes. Create a root page HTML file, a part under `parts/`, or CSS/JS under `assets/`. Required files, known literal references and material/revision template references block rename/delete. Dynamic references cannot be proven unused; review custom JavaScript and template logic. Removed files retain one raw recovery copy under template history. ZIP export contains saved files, respects the installer limits and does not activate anything.

Compare shows every changed line as one contiguous hunk with context, without an expensive quadratic diff. Private Preview posts unsaved source into a temporary owner-bound preview: at most 16 entries, 512 KiB of source each, expiring after ten minutes. Preview uses real published content, private asset URLs and noindex; it neither saves nor activates the template. Unsaved CSS/JS is visible when the template references that asset through `.ThemeAssets`. A restart or expiration discards previews.

Reset is available for built-ins. It checks a hash of the entire current file set, removes the local override and retains one previous local directory in `theme/editor-history/<template-hash>/before-reset` for manual recovery. Content and activation are preserved. Export or back up valuable local edits first.

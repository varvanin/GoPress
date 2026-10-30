# GoPress

GoPress is a self-hosted CMS for one site per installation, built with Go, Echo, SQLite, templ, HTMX and Tiptap. It runs without a separate database server, CDN, Redis or cloud account. Project code is Apache-2.0; bundled components retain their own licenses.

**Development build, not a stable public release.** Automated Go, SQLite, HTTP and frontend checks are available. Browser/device acceptance, a real Docker/CI run, external SMTP delivery and production-domain verification remain separate release requirements. Cross-compilation alone does not establish platform support.

<!-- admin-preview:start -->
## Admin interface

[Open the English admin preview](docs/admin-preview.html). The preview uses an isolated demonstration site, not production data.
<!-- admin-preview:end -->

## Quick start

Development uses Go 1.27.1 and Node.js 24.14.1/npm. The compiled binary embeds the admin, public templates, CSS/JavaScript, developer references, license notices and timezone database. An installed site does not need Go or Node.js.

```sh
make build
./bin/gopress serve --data ./data --addr 127.0.0.1:8080
```

Open `http://127.0.0.1:8080/install`. Enter the random installation token printed in this process's console, a site name, the base URL `http://127.0.0.1:8080`, an IANA timezone such as `Europe/Warsaw`, your email and a password of 12–72 bytes. Then sign in at `/login`.

There is no fixed administrator account. Installation is transactional and cannot be repeated. Restarting an incomplete installation produces a new token; completed installations do not expose another token. An existing corrupt or unsupported database is never treated as a fresh site.

The base URL is an origin without a path, query or fragment. Use the same origin for installation and sign-in. HTTPS enables Secure cookies. Proxy headers are trusted only when explicitly configured; see [operations](docs/OPERATIONS.md).

Flags override environment values:

| Flag | Environment | Default |
| --- | --- | --- |
| `--data` | `GOPRESS_DATA` | `./data` |
| `--addr` | `GOPRESS_ADDR` | `127.0.0.1:8080` |

Keep data on a local persistent disk, not NFS/SMB or a public web directory. Only one process may hold the site's directory lock. Shutdown with Ctrl+C/SIGTERM drains requests and closes SQLite. `/healthz` checks the process; `/readyz` checks the database, including before installation.

The current local verification platform is macOS amd64. Linux amd64/arm64 packages can be built; deployment acceptance is still required. The filesystem lock currently makes Windows unsupported.

## Build and verify

```sh
npm ci
go install github.com/a-h/templ/cmd/templ@v0.3.1070
make release TEMPL="$(go env GOPATH)/bin/templ"
make test
make check
```

- `make generate` checks the exact templ version and regenerates Go templates.
- `npm run build:css` builds Tailwind; `npm run build:editor` bundles the editors, admin runtime and dependency license notices.
- `npm run build` builds both frontend parts. Dependencies are pinned by `package-lock.json`.
- `make release` builds assets, generates templ and compiles Go. `make build` compiles using the existing embedded assets.
- `make check` checks formatting, frontend tests, `go vet` and Go race tests. Node tests are not browser acceptance.

After rebuilding, stop the old process yourself and restart it with the same `--data` directory. Refreshing a browser cannot replace CSS/JS/templ embedded in an already running binary.

Restricted development environments may need an alternate writable `GOCACHE`, such as `/tmp/gopress-go-cache`. Such a cache is not part of the project or a normal installation requirement. Do not mistake offline dependency checks for a completed online vulnerability or checksum-database audit.

## First end-to-end acceptance

After `make release`, run the isolated live-process acceptance test:

```sh
node scripts/smoke.mjs
node scripts/smoke.mjs --browser
# Optional container acceptance, after building the image:
docker build -t gopress:ci .
node scripts/smoke.mjs --docker
```

The test uses temporary data and random credentials. It covers protected concurrent installation, sign-in, image upload, draft/authorized preview, publication, draft isolation, restart, offline CLI backup/verification and restoration into a separate directory. It launches the restored site, compares the published HTML and image bytes, checks the retained working revision and requires fresh sign-in after restoration. It also checks rejected uploads, CSRF, URL collisions, forms, search and maintenance. It never uses the working site's database.

A free loopback TCP port is required. `--browser` additionally requires Chrome (override with `GOPRESS_BROWSER`) and checks real editor typing/autosave and mobile width; `--docker` requires Docker and the prebuilt image. The HTTP integration suite runs against temporary SQLite without listening sockets, but does not replace these live-process/browser checks. See [verified progress and limitations](docs/PROGRESS.md).

## Local demonstration content

With the local server stopped, add demonstration content to an installed site:

```sh
go run scripts/seed-demo.go --data ./data
./bin/gopress serve --data ./data --addr 127.0.0.1:8080
```

This publishes six pages (including nested pages) and eight articles, adds two drafts, a post scheduled one week ahead and a trashed post, and creates three original PNG/JPEG illustrations. It adds categories, tags and navigation links while preserving existing materials, accounts, settings and menu items. One published article also has a newer private working revision for publication-isolation testing. Sample URLs use a `demo-` prefix; contact details are illustrative.

The script first creates and verifies an offline backup under the data directory's sibling `backups` folder. It requires the exclusive data-directory lock, so it refuses to run against the active server. It checks public pages, image decoding, draft privacy and SQLite integrity through the real handlers. A repeat run reuses existing demo paths and media. The report and media manifest are stored as `demo-content-report.json` and `demo-media.json` in the data folder. Use this only for local testing: the published materials and scheduled publication are real site data.

## Features

### Content and publishing

Pages and posts have drafts, published snapshots, version checks, revisions, autosave, previews, scheduling, trash and bulk actions. Pages support parents, ordering, custom templates, nested URLs and permanent redirects after published address changes. Posts support categories and tags.

The Tiptap editor supports section anchors, headings, lists, quotes, links, inline/code blocks, image captions and alt text, alignment, common text marks, undo/redo and search. Autosave serializes requests after a 1.5-second pause. Conflicts or expired sessions stop retries without replacing local text. Unsaved navigation is guarded. There is no browser-disk offline draft store.

For a one-page site, select a heading or paragraph and click **Anchor** in the text toolbar. Give it a unique lowercase name such as `contacts`; the panel provides `#section-contacts`. Use that URL in the existing Link tool or as a custom menu link. Use `/#section-contacts` in a shared menu targeting the homepage, or `/about#section-contacts` for another page. Anchor names allow up to 64 ASCII letters, digits and hyphens, starting with a letter. They are explicit and remain stable when the heading text changes. **Remove anchor** removes the target; update any links to it manually. Copies pasted into the editor and newly split paragraphs do not duplicate the original anchor. Drafts, translations, revisions and publication snapshots keep their own anchors with their document. The `section-` prefix separates content targets from template IDs. Custom templates should avoid IDs starting with `section-` and can add scroll margin for fixed headers.


Pages and posts start with one content language of their own, independently of the admin language. The **Content language** switcher in the editor’s settings column shows only versions already belonging to that material. Use **Add language** to create another version, then write its title, body and SEO. Once versions have been added, the switcher lists only those versions. The URL slug, cover, categories, template and publication status are shared. Switching preserves local edits and resets undo history across languages. Saving, revisions, scheduled publication and backups include all versions. An added version without a title stays in the draft and is excluded from publication.

A published translation is available at the existing URL with a language parameter, for example `/about?lang=en`; an unavailable translation returns 404. Preview uses the selected language. This does not translate template labels, navigation, feeds or search results, and does not add a public language menu or language-prefixed routes. The original remains at `/about`. See [content languages](docs/CONTENT_LANGUAGES.md).

Publishing is atomic. Saving a newer draft does not change the published body, URL, cover or SEO. Scheduled publishing pins a particular revision; later draft edits cannot replace it. Times are entered in the site's timezone. The worker catches up after downtime and checks every 15 seconds; ambiguous or nonexistent local times are rejected. Each due job is isolated. A parent/URL conflict stops that job, preserves its draft and pinned revision, and reports the failure in the editor and action log; fix the conflict and schedule it again.

Documents use a strict shared JSON schema, not arbitrary client HTML: up to 1 MiB per language, 4 MiB across all language versions, 10,000 nodes per document and depth 32. Content writes accept at most 32 MiB of encoded request data to allow JSON/form escaping; the smaller document limits are still enforced. Titles are limited to 200 Unicode code points. Links are restricted to supported schemes and local/anchor URLs. Inline images must reference existing uploads.

### Media and menus

The media library and editor picker support search and pagination. PNG/JPEG uploads are checked before decoding, limited to 8 MiB and 16 million pixels, re-encoded and stored under random names. Existing references in drafts, publications and revisions protect used media from deletion. File details include title, alt text and caption, with revision checks against concurrent edits. Used-file protection also covers SEO images, site SEO settings and module settings, including same-origin URLs with query strings.

Header and footer menus support ordered links and up to three levels. Removing a parent preserves its children. Menus use explicit links; review a menu link when changing the linked page's URL.

### Templates and the file editor

Journal, Minimal and Studio are distinct built-in templates. Install custom template ZIPs, preview without activation and switch without changing content. The catalog clearly identifies the active template. Built-in previews are design illustrations rather than browser screenshots.

The in-app reference documents template selection, shared parts, variables, functions and conditions. The CodeMirror file editor supports HTML/CSS/JS, Go directives, line numbers, search, wrapping and Ctrl/Cmd+S. Built-ins are read-only until copied. Saves validate the HTML template set, check the file hash and atomically replace a file; the previous version can be restored.

[Template guide](docs/THEMES.md) · [Developer reference](internal/httpui/starter/TEMPLATE_REFERENCE.md) · [Journal](docs/JOURNAL_DESIGN.md) · [Minimal](docs/MINIMAL_DESIGN.md)

### Forms and modules

Build forms with text, email, phone, textarea, select and checkbox fields. Insert `[form id=1]` as a separate paragraph in a page or post, or use `/forms/1`. Disabled forms do not accept submissions.

Submissions are saved before email delivery. Background notifications use bounded retries and submission deduplication. Admin tools provide delivery status, retry, CSV export, bulk deletion and retention. SMTP delivery is at least once: an acknowledgement failure after delivery may produce another email.

Modules include the built-in FAQ: question groups, ordering and native expandable answers, embedded using `[faq id=1]`. Custom modules are configurable HTML/CSS ZIP packages with settings and optional previews; insert `[module id="contact-card"]`. Multiple modules can be enabled, and disabling preserves their settings. They do not execute arbitrary Go/PHP server code.

Built-in Gallery and Testimonials manage ordered image collections and manually entered reviews. Choose images from the media library, add captions or names/company/text, and insert `[gallery id=1]` or `[testimonials id=1]` as a separate paragraph. Testimonials support grid/list layouts. Disabling preserves groups; used images are protected from deletion. Group edits update their published blocks immediately.

[Module format and starter](docs/MODULES.md)

### Search and SEO

Public search at `/search` reads published FTS snapshots only. Rebuild the index with the CLI when required.

SEO has five sections: search title rules and descriptions, indexing/canonical/robots directives, Open Graph and X/Twitter cards, JSON-LD and ownership verification, and sitemap/robots.txt/RSS controls. Individual metadata belongs to the published content snapshot. The sitemap excludes non-indexable content and supports explicit exclusions and chunking; XML responses support ETag/304.

[SEO settings and priorities](docs/SEO.md)

### Administration and appearance

Administrators manage users and the site; editors manage content and media; authors work with their own posts/media without publishing. Server-side permissions do not rely on hidden buttons. Blocking, password changes and access recovery revoke sessions; the last active administrator is protected.

General, home-page, mail and language settings are separate. The admin and sign-in support English, Russian, Polish, German, Ukrainian, Italian, Spanish and Czech; changing the interface language does not translate user content or public templates.

The admin has dark and gray/white light appearances, with blue active navigation in light mode. Switch at the bottom of the sidebar or on system pages. The browser remembers the choice; public templates remain independent. See [design rules](docs/ADMIN_STYLE.md) and [contributor instructions](AGENTS.md).

## Mail configuration

Configure SMTP under Settings → Mail, then send a test message. A blank password keeps the existing password; removal is explicit. File settings are stored atomically in `<data>/mail.json` with mode `0600`, never returned in HTML and excluded from standard backups.

Server environment overrides admin mail settings:

```sh
export GOPRESS_SMTP_HOST=smtp.example.com
export GOPRESS_SMTP_PORT=587
export GOPRESS_SMTP_FROM=website@example.com
export GOPRESS_SMTP_USER=website@example.com
export GOPRESS_SMTP_PASSWORD='your-smtp-password'
./bin/gopress serve
```

Port 587 requires STARTTLS; port 465 uses TLS from connection start. Certificates are verified. Omit username/password for a relay that does not require authentication. Each form has its own recipient. Restore SMTP configuration separately after moving a site.

## Backups, maintenance and recovery

The admin Tools section manages backup creation, import, verification, download, deletion, restoration, diagnostics, maintenance and the action log. Maintenance serves a system-styled 503 page with `Retry-After`, `no-store` and `noindex`; sign-in and tools remain available.

For offline directory backups, stop the server first. Parent directories must exist; the destination must not already exist and must be outside the live data directory.

```sh
mkdir -p ../gopress-backups
./bin/gopress backup create --data ./data --output ../gopress-backups/site-001
./bin/gopress backup verify --source ../gopress-backups/site-001
./bin/gopress backup restore --source ../gopress-backups/site-001 --data ./restored-data
./bin/gopress serve --data ./restored-data --addr 127.0.0.1:8080
```

Backups include a consistent SQLite snapshot, uploads, installed templates/editor history and SHA-256 manifests. Module packages/settings are in SQLite. Secrets and environment settings are separate. Restore revokes sessions. Interrupted replacement fails closed; do not remove recovery markers blindly.

[Operations, trusted proxies, recovery and release checks](docs/OPERATIONS.md)

## AI writing assistant

Open **Settings → AI**, save an OpenRouter API key and a model ID such as `openrouter/auto`. Only administrators can change these settings. The key is stored in `ai.json` with owner-only permissions, outside SQLite and ordinary backups; configure it again after restoring to another installation. Keep the data directory private as with SMTP credentials.

Pages and posts have an **AI** panel. Enter a task to draft, continue, rewrite or translate text. The task, title and active material language are sent to [OpenRouter](https://openrouter.ai/docs/api/api-reference/chat/create-a-chat-completion); existing text is included only when **Include existing text** is checked. The interface language does not control the writing language. Review the plain-text result, then explicitly append it to the current language version. The helper never publishes or automatically replaces content; normal editing, undo and autosave apply after insertion. Results are temporary and disappear when leaving the editor.

Administrators, editors and authors can use the helper within their existing editing permissions. Requests use a fixed HTTPS endpoint, a 75-second timeout, one concurrent generation per site, at most 30 attempts per user per 15 minutes and a 4096-token output ceiling. Context is limited to 64,000 UTF-8 bytes; large materials can be generated without context. Charges and model availability depend on your OpenRouter account. Tests use mocked responses and never spend API credits.

## Updates through Dokploy

**Settings → Updates** checks for a signed stable release and offers an **Update** button. Installation is opt-in; there is no scheduled installation or background release polling. This first integration supports an official Docker image deployed as a Dokploy **Application** with a Docker source, rather than a Dokploy Compose project or a native binary.

The publisher must first publish signed releases and public GHCR images; see [.github/RELEASING.md](.github/RELEASING.md). Development builds do not ship a production signing key. A button cannot install an unpublished release.

In Dokploy, keep one replica, mount a persistent named volume or bind directory at `/data`, use local storage and pin the application to its data node in Advanced → Placement (for example `node.hostname==site-node`). Keep the image’s default entrypoint, command and port 8080; custom command/argument profiles are rejected by this first updater. Restrict the Dokploy API key to the GoPress application where possible. Set these variables in the application's environment, with the actual application ID unquoted:

```dotenv
GOPRESS_DOKPLOY_URL=https://dokploy.example.com
GOPRESS_DOKPLOY_API_KEY=YOUR_DOKPLOY_API_KEY
GOPRESS_DOKPLOY_APPLICATION_ID=YOUR_APPLICATION_ID
```

Official release images embed the release feed and public signing key. For a bootstrap build or a private release channel, the operator may set `GOPRESS_UPDATE_PUBLIC_KEY` (base64 Ed25519 public key) and `GOPRESS_UPDATE_FEED` (HTTPS URL of `update.json`, with a detached base64 signature at `update.json.sig`). The feed is consulted only on administrator request and is never allowed to nominate an image outside the image repository compiled into that build. Never put the private release signing key in a site, Docker build argument or container environment.

The update job rechecks the signed release, checks the Dokploy target and persistent storage, freezes writes, creates and verifies a backup, and requests deployment of an immutable image digest. It enforces `stop-first`, one replicated task, paused deployment failures, disabled automatic rollback and a local readiness probe. Site secrets stay in Dokploy's environment; no Docker socket or root access is given to GoPress. Brief downtime is expected.

The new process removes the update maintenance lock only after binding its listener, checking SQLite integrity and matching the expected application version/schema. A pre-existing operator maintenance state is retained. A queued deployment is not reported as installed. If Dokploy times out or fails after a remote mutation, GoPress keeps maintenance and blocks background writes; inspect Dokploy before taking further action. Status polling lasts at most five minutes; it does not retry deployment.

For recovery, stop the failed application in Dokploy. Keep the recorded previous image reference and verified pre-update backup. Restore compatible data using the documented offline backup recovery procedure, select the compatible previous image in Dokploy, then check the restored site. After verification, remove `update-in-progress` and archive/remove `updates/state.json` while the application is stopped; leave ordinary maintenance enabled until the checks are complete. Do not simply roll back an image against a migrated database. Backups and environment secrets remain separate. See [operations](docs/OPERATIONS.md).

## Internal HTTP interface

These are implementation endpoints, not a stable external SDK. Mutations require a session, CSRF and applicable role/object permissions. A token is available from authenticated `GET /admin/csrf`; setup/sign-in forms contain a hidden `csrf`. GET never mutates state.

| Endpoint | Purpose |
| --- | --- |
| `GET /admin/pages`, `/admin/posts` | Paginated metadata, `q`, `page`, `trash`, term filters; HTML or JSON |
| `GET /admin/pages/{id}` | Full working document |
| `POST /admin/pages/save` | Browser save/publish/autosave with optimistic version |
| `POST /admin/pages`, `/admin/pages/{id}` | Internal JSON content writes |
| `POST /admin/pages/ai` | Generate plain text for an authorized editor; does not save content |
| `GET /admin/ai`, `POST /admin/ai/settings` | Administrator-only OpenRouter configuration |
| `GET /admin/updates`, `/admin/updates/status` | Administrator-only update metadata and deployment status |
| `POST /admin/updates/check`, `/admin/updates/install` | Verify a release or start an explicit Dokploy update job |
| `POST /admin/media` | Raw PNG/JPEG bytes, not multipart; returns the stored name |
| `GET /admin/pages/{id}/preview` | Authorized, non-cacheable draft preview |
| `POST /admin/pages/{id}/publish`, `/schedule`, `/state` | Versioned publication and lifecycle changes |
| `POST /admin/content/bulk` | Bulk content actions |
| `POST /logout` | Revoke the current session |

HTMX preserves the sidebar and replaces the relevant main/panel fragment. History, errors and unsaved changes must remain safe. See the implementation and tests for the complete route set.

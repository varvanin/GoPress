# Custom GoPress modules

A module is a ZIP containing `module.json`, `template.html` and optional `style.css`, `screenshot.png`/`screenshot.jpg` and `README.md`. Place files at the root or in one common folder. Upload under Modules → Upload your module, configure, enable and insert `[module id="contact-card"]` as a separate paragraph in a page or post.

## module.json

`id` is unique: a lowercase ASCII letter followed by letters, digits or hyphens, at most 48 characters. `faq`, `gallery` and `testimonials` are reserved for built-in modules. `name`, `version`, `author` and `description` appear in the catalog. `fields` defines up to 20 settings.

A field has `key` (a letter followed by letters/digits/underscores, at most 40 characters), `label`, `type` (`text`, `textarea`, `url`) and optional `default` / `required`. Values are strings up to 5,000 characters; URLs must be absolute HTTP/HTTPS. Supply valid initial values for required fields. Settings saves use optimistic version checks.

## template.html

Standard Go `html/template` escapes dynamic values automatically.

| Expression | Meaning |
| --- | --- |
| `{{.ID}}` | Package identifier |
| `{{.Settings.title}}` | The `title` setting |
| `{{if .Settings.link}}...{{else}}...{{end}}` | Conditional output |
| `{{range $key, $value := .Settings}}{{$key}}: {{$value}}{{end}}` | Iterate over settings |

Supported operations include `eq`, `ne`, `lt`, `le`, `gt`, `ge`, `and`, `or`, `not`, `len` and `index`. There is no database, filesystem, network, server-function or private-user access. Template/block calls and formatting functions are rejected to prevent recursion and unbounded allocations. `range` may iterate only over `.Settings`, without nested loops. Syntax and missing-setting errors are checked before installation/enabling; expression errors include the file and line. Output is limited to 256 KiB.

CSS loads from `/module-assets/<id>/style.css`. The block is wrapped in `.gp-module-<id>`: scope your selectors to it and inherit the public template's colors/fonts. Previews are PNG/JPEG, at most 2 MiB and 4,000 px; without one, a neutral illustration is shown. Admin previews are images, not executable package HTML.

## Installation and limits

Limits: 8 MiB ZIP, 16 MiB unpacked, 32 files/directories, and 128 KiB each for HTML/CSS. Traversal, links and unknown files are rejected. Packages/settings live in SQLite and are included in backups. New packages start disabled. Disabling retains settings; deletion is permanent and allowed only while disabled. Installing the same ID again is rejected: retain settings and remove the disabled package before replacing it.

This format provides configurable public blocks, not arbitrary server extensions. Go/PHP, request handlers, background tasks and JavaScript files are not supported. An author's static HTML/CSS is trusted site content: install trusted packages. The site's CSP still applies.

The built-in FAQ remains separate and uses `[faq id=1]`; installing a package does not change its groups. Multiple modules can be enabled simultaneously.

Public pages allow at most 50 blocks of each shortcode kind and enforce an 8 MiB assembled-document limit during expansion. A custom module keeps its individual 256 KiB output limit.

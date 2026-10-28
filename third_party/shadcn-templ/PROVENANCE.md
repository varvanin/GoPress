# shadcn-templ provenance

Source: https://github.com/axadrn/shadcn-templ/tree/v2.0.0-beta.15 (MIT).

The CMS dashboard adapts dashboard-01: Sidebar.Provider → Sidebar → Inset → SiteHeader, Card summaries and Table. Demonstration charts, customers, revenue and placeholder navigation were not copied. Metrics come from SQLite.

| Source file | Original Git blob SHA |
| --- | --- |
| blocks/dashboard01/page.templ | 91dd06f5803b9d3e4a5c62b2e1c3f725b5b10609 |
| blocks/dashboard01/app_sidebar.templ | 072a0fb7dce6a06bdd9e582d953befe15f525cd4 |
| blocks/dashboard01/site_header.templ | 56e8553f612ac333d9f1b0535fee0259474bcdaa |
| blocks/dashboard01/section_cards.templ | 8d63de8eee78a18d49f3143f3aeffac747a729d9 |
| components/sidebar/sidebar.templ | dd3a324ff777f83a222ef7b8966cc7f438cb1e35 |
| components/sidebar/sidebar.js | 46dc6a2c97743fb9a2e29b3a1ea87bd18c09fcec |
| components/card/card.templ | 671663808921ac90e0f9a04171db42c371d3189a |
| components/table/table.templ | 4e4743c0e44f3cda6c84656dcb258b66c24a2c8b |
| components/badge/badge.templ | a4ff882cb5007a48a585a920c0a9bca00fee8cc3 |
| components/sheet/sheet.templ | 5e4a2aebbca2b9ed3fe7a787739001b77ec1d4e6 |
| components/separator/separator.templ | 7f3d30372526b35c5841cf131a14509ef5263f7d |
| components/skeleton/skeleton.templ | a1f37dc5bab71aca85b0ded76bdd77ecfafc0294 |
| components/tooltip/tooltip.templ | 5dcc5cb83dabf0812392b2a9ccc2d87ba7857667 |
| components/icon/icon.go | 9e45a13fdcac5fc2c9ae468860c9c86efea94d41 |
| components/icon/icon_defs.go | 24105b3785efcd0798295f34d6840e0382d83734 |
| components/icon/icon_data.go | b2a04af31f33d0bc4db7c116d2ed005f58d92055 |

Local adaptations:

- Module imports point to gopress/internal/ui; templates formatted with templ 0.3.1070.
- Sidebar.Provider/mobile width variables and Sheet backdrop user-select moved from inline attributes into external CSS, preserving CSP.
- Sidebar keyboard shortcut ignores prevented events and focused inputs/textareas/contenteditable, so Tiptap bold is unaffected.
- Only used Lucide 0.576.0 icon definitions/SVG paths are retained, from the original icon package. ISC notice is in third_party/lucide.
- Original Nova sections for Badge, Button, Card, Input, Sheet, Sidebar, Skeleton, Table and Tooltip are retained. The stylesheet is imported into Tailwind's components layer; app utilities can override component sizes normally.
- The original dialog/sidebar and required Base UI hooks are bundled locally, with MIT notices from shadcn-templ and Base UI. No React runtime, remote registry or CDN is needed by the installed CMS.


## Select

Original Select from the same v2.0.0-beta.15 tag:

| File | Git blob SHA |
| --- | --- |
| components/select/select.templ | f8b25df82ce1a42be245b2332e031e2bfd10c93f |
| components/select/select.js | acf0ec3514dc8ab4e44fc2f1ddb74375d93d7c52 |
| components/baseui/use_list_navigation.js | 3ad641d06393fb627877886f020ae38c04e52bab |
| components/baseui/use_typeahead.js | bf6ad0a482336e99325c5df3dce6c56a13c8aca4 |
| components/baseui/use_anchor_positioning.js | 909fc943c3c3b2bcc9d19f567620a10b9039df3a |
| components/baseui/internal_backdrop.js | 024a94e957708246f9efe35108402ca74e1323da |
| components/baseui/composite.js | ac2cc2da0d7adf95ed602154aee0c43145bfdb8e |
| components/baseui/direction.js | 27fd33930c0a4ac657e6d01209724c51fecf7b04 |

Adaptations: module imports use the local UI package; existing ChevronRight renders rotated chevrons; the form value uses type=hidden instead of an inline-styled text input (autofill is not needed for these enum controls); static scroll-arrow positioning is in external CSS. Select.js keyboard/typeahead, dismissal, portal and lifecycle behavior are retained. Required Floating UI sources and their MIT notice are recorded in third_party/floating-ui. All scripts are bundled locally in dependency order. Menu-row cloning renews trigger/popup/list IDs before insertion; values are submitted under the original field names. No React or CDN is used.

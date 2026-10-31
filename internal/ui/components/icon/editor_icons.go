package icon

// Additional toolbar SVGs from lucide v1.8.0 (ISC); see third_party/lucide.
func init() {
	internalSvgData["table"] = `<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18M3 15h18M9 3v18M15 3v18"/>`
	internalSvgData["hash"] = `<path d="M4 9h16M3 15h16M10 3 8 21M16 3l-2 18"/>`

	internalSvgData["undo-2"] = `<path d="M9 14 4 9l5-5" />
<path d="M4 9h10.5a5.5 5.5 0 0 1 5.5 5.5a5.5 5.5 0 0 1-5.5 5.5H11" />`
	internalSvgData["redo-2"] = `<path d="m15 14 5-5-5-5" />
<path d="M20 9H9.5A5.5 5.5 0 0 0 4 14.5A5.5 5.5 0 0 0 9.5 20H13" />`
	internalSvgData["heading"] = `<path d="M6 12h12" />
<path d="M6 20V4" />
<path d="M18 20V4" />`
	internalSvgData["list"] = `<path d="M3 5h.01" />
<path d="M3 12h.01" />
<path d="M3 19h.01" />
<path d="M8 5h13" />
<path d="M8 12h13" />
<path d="M8 19h13" />`
	internalSvgData["list-ordered"] = `<path d="M11 5h10" />
<path d="M11 12h10" />
<path d="M11 19h10" />
<path d="M4 4h1v5" />
<path d="M4 9h2" />
<path d="M6.5 20H3.4c0-1 2.6-1.925 2.6-3.5a1.5 1.5 0 0 0-2.6-1.02" />`
	internalSvgData["list-indent-increase"] = `<path d="M21 5H11" />
<path d="M21 12H11" />
<path d="M21 19H11" />
<path d="m3 8 4 4-4 4" />`
	internalSvgData["square-code"] = `<path d="m10 9-3 3 3 3" />
<path d="m14 15 3-3-3-3" />
<rect x="3" y="3" width="18" height="18" rx="2" />`
	internalSvgData["bold"] = `<path d="M6 12h9a4 4 0 0 1 0 8H7a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1h7a4 4 0 0 1 0 8" />`
	internalSvgData["italic"] = `<line x1="19" x2="10" y1="4" y2="4" />
<line x1="14" x2="5" y1="20" y2="20" />
<line x1="15" x2="9" y1="4" y2="20" />`
	internalSvgData["strikethrough"] = `<path d="M16 4H9a3 3 0 0 0-2.83 4" />
<path d="M14 12a4 4 0 0 1 0 8H6" />
<line x1="4" x2="20" y1="12" y2="12" />`
	internalSvgData["code-xml"] = `<path d="m18 16 4-4-4-4" />
<path d="m6 8-4 4 4 4" />
<path d="m14.5 4-5 16" />`
	internalSvgData["underline"] = `<path d="M6 4v6a6 6 0 0 0 12 0V4" />
<line x1="4" x2="20" y1="20" y2="20" />`
	internalSvgData["highlighter"] = `<path d="m9 11-6 6v3h9l3-3" />
<path d="m22 12-4.6 4.6a2 2 0 0 1-2.8 0l-5.2-5.2a2 2 0 0 1 0-2.8L14 4" />`
	internalSvgData["link"] = `<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
<path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />`
	internalSvgData["superscript"] = `<path d="m4 19 8-8" />
<path d="m12 19-8-8" />
<path d="M20 12h-4c0-1.5.442-2 1.5-2.5S20 8.334 20 7.002c0-.472-.17-.93-.484-1.29a2.105 2.105 0 0 0-2.617-.436c-.42.239-.738.614-.899 1.06" />`
	internalSvgData["subscript"] = `<path d="m4 5 8 8" />
<path d="m12 5-8 8" />
<path d="M20 19h-4c0-1.5.44-2 1.5-2.5S20 15.33 20 14c0-.47-.17-.93-.48-1.29a2.11 2.11 0 0 0-2.62-.44c-.42.24-.74.62-.9 1.07" />`
	internalSvgData["align-left"] = `<path d="M21 5H3" />
<path d="M15 12H3" />
<path d="M17 19H3" />`
	internalSvgData["align-center"] = `<path d="M21 5H3" />
<path d="M17 12H7" />
<path d="M19 19H5" />`
	internalSvgData["align-right"] = `<path d="M21 5H3" />
<path d="M21 12H9" />
<path d="M21 19H7" />`
	internalSvgData["align-justify"] = `<path d="M3 5h18" />
<path d="M3 12h18" />
<path d="M3 19h18" />`
	internalSvgData["image-plus"] = `<path d="M16 5h6" />
<path d="M19 2v6" />
<path d="M21 11.5V19a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h7.5" />
<path d="m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21" />
<circle cx="9" cy="9" r="2" />`
	internalSvgData["search"] = `<path d="m21 21-4.34-4.34" />
<circle cx="11" cy="11" r="8" />`
	internalSvgData["quote"] = `<path d="M16 3a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2 1 1 0 0 1 1 1v1a2 2 0 0 1-2 2 1 1 0 0 0-1 1v2a1 1 0 0 0 1 1 6 6 0 0 0 6-6V5a2 2 0 0 0-2-2z" />
<path d="M5 3a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2 1 1 0 0 1 1 1v1a2 2 0 0 1-2 2 1 1 0 0 0-1 1v2a1 1 0 0 0 1 1 6 6 0 0 0 6-6V5a2 2 0 0 0-2-2z" />`
	internalSvgData["minus"] = `<path d="M5 12h14" />`
	internalSvgData["pilcrow"] = `<path d="M13 4v16" />
<path d="M17 4v16" />
<path d="M19 4H9.5a4.5 4.5 0 0 0 0 9H13" />`
}

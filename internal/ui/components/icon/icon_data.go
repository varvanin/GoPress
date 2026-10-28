package icon

const LucideVersion = "0.576.0"

var internalSvgData = map[string]string{
	"command": `<path d="M15 6v12a3 3 0 1 0 3-3H6a3 3 0 1 0 3 3V6a3 3 0 1 0-3 3h12a3 3 0 1 0-3-3" />`,
	"panel-left": `<rect width="18" height="18" x="3" y="3" rx="2" />
  <path d="M9 3v18" />`,
	"x": `<path d="M18 6 6 18" />
  <path d="m6 6 12 12" />`,
	"layout-dashboard": `<rect width="7" height="9" x="3" y="3" rx="1" />
  <rect width="7" height="5" x="14" y="3" rx="1" />
  <rect width="7" height="9" x="14" y="12" rx="1" />
  <rect width="7" height="5" x="3" y="16" rx="1" />`,
	"file-text": `<path d="M6 22a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h8a2.4 2.4 0 0 1 1.704.706l3.588 3.588A2.4 2.4 0 0 1 20 8v12a2 2 0 0 1-2 2z" />
  <path d="M14 2v5a1 1 0 0 0 1 1h5" />
  <path d="M10 9H8" />
  <path d="M16 13H8" />
  <path d="M16 17H8" />`,
	"plus": `<path d="M5 12h14" />
  <path d="M12 5v14" />`,
	"arrow-up-right": `<path d="M7 7h10v10" />
  <path d="M7 17 17 7" />`,
	"arrow-right": `<path d="M5 12h14" />
  <path d="m12 5 7 7-7 7" />`,
	"arrow-left": `<path d="m12 19-7-7 7-7" />
  <path d="M19 12H5" />`,
	"chevron-right": `<path d="m9 18 6-6-6-6" />`,
	"log-out": `<path d="m16 17 5-5-5-5" />
  <path d="M21 12H9" />
  <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />`,
	"globe": `<circle cx="12" cy="12" r="10" />
  <path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20" />
  <path d="M2 12h20" />`,
	"check": `<path d="M20 6 9 17l-5-5" />`,
	"circle-check": `<circle cx="12" cy="12" r="10" />
  <path d="m9 12 2 2 4-4" />`,
	"pen-line": `<path d="M13 21h8" />
  <path d="M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z" />`,
	"image": `<rect width="18" height="18" x="3" y="3" rx="2" ry="2" />
  <circle cx="9" cy="9" r="2" />
  <path d="m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21" />`,
	"eye": `<path d="M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 0 1 0 .696 10.75 10.75 0 0 1-19.876 0" />
  <circle cx="12" cy="12" r="3" />`,
	"save": `<path d="M15.2 3a2 2 0 0 1 1.4.6l3.8 3.8a2 2 0 0 1 .6 1.4V19a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z" />
  <path d="M17 21v-7a1 1 0 0 0-1-1H8a1 1 0 0 0-1 1v7" />
  <path d="M7 3v4a1 1 0 0 0 1 1h7" />`,
	"send": `<path d="M14.536 21.686a.5.5 0 0 0 .937-.024l6.5-19a.496.496 0 0 0-.635-.635l-19 6.5a.5.5 0 0 0-.024.937l7.93 3.18a2 2 0 0 1 1.112 1.11z" />
  <path d="m21.854 2.147-10.94 10.939" />`,
	"lock-keyhole": `<circle cx="12" cy="16" r="1" />
  <rect x="3" y="10" width="18" height="12" rx="2" />
  <path d="M7 10V7a5 5 0 0 1 10 0v3" />`,
	"check-check": `<path d="M18 6 7 17l-5-5" />
  <path d="m22 10-7.5 7.5L13 16" />`,
	"arrow-down": `<path d="M12 5v14" />
  <path d="m19 12-7 7-7-7" />`,
	"circle": `<circle cx="12" cy="12" r="10" />`,
}

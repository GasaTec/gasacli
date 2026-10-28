# GasaCLI Design System

Status: visual source of truth for product prompts. Product features in these screens are not evidence of implementation.

## Brand

Use the supplied `docs/img/logo.png` as the canonical mark and wordmark reference. Preserve its navy G and teal chevron prompt; do not redraw, recolor, distort, crop, rotate, add effects, or place it on low-contrast backgrounds. Keep clear space equal to the height of the G on every side. Minimum mark size is 24 px on web and 2 terminal cells; minimum wordmark lockup is 112 px wide on web. If the asset is unavailable in a constrained TUI, render a simple navy/teal G-chevron ASCII approximation and spell `gasacli` in lowercase. Use a monochrome navy or white mark only when the background requires it, retaining the chevron shape.

Brand anchors: navy `#172A46`, teal `#1ECFA5`. Navy is the primary brand ground; teal signals ready/active/focus and is paired with dark ink for text on a teal fill.

## Color tokens

| Token | Dark | Light | Use |
|---|---|---|---|
| background | `#0B1220` | `#F4F7FB` | App canvas |
| surface | `#111C2E` | `#FFFFFF` | Cards, panes |
| elevated | `#1B2A40` | `#FFFFFF` | Menus, dialogs, hover surfaces |
| border | `#596982` | `#7F8CA0` | Visible dividers and component edges |
| text-primary | `#F4F7FB` | `#172A46` | Main text |
| text-secondary | `#C3CFDF` | `#44546A` | Supporting text |
| text-muted | `#9AAAC0` | `#5B6B80` | Hints and metadata; never disabled-only information |
| accent | `#1ECFA5` | `#087E68` | Focus, links, primary actions; on accent fill use navy ink in dark mode and white in light mode |
| success | `#43D19E` | `#137A54` | Verified success, done |
| warning | `#F2B84B` | `#8A5600` | Approval needed, working |
| danger | `#FF7272` | `#B4232F` | Failure, destructive action |
| info | `#79B8FF` | `#145FA8` | Neutral guidance |
| status-working | `#F2B84B` | `#8A5600` | Active worker / running |
| status-done | `#43D19E` | `#137A54` | Verified done |
| status-idle | `#9AA3B2` | `#667085` | Idle / no active work |
| status-error | `#FF7272` | `#B4232F` | Failed / blocked by error |
| status-ready | `#1ECFA5` | `#087E68` | Ready / available |

Status is always conveyed by a word or icon as well as color. `working` means active work, not verified completion. `done` means the applicable gates passed. `idle` means no active work. `error` means an operation failed. `ready` means available to act. `BLOCKED` and `NEEDS_APPROVAL` are distinct labeled states; do not imply either is done.

## WCAG contrast checks

Ratios below use WCAG 2.2 relative luminance, rounded to two decimals. Text and meaningful icons meet AA 4.5:1; large text meets AA 3:1. Component boundaries and focus indicators meet 3:1 against adjacent surfaces. Never put white body text directly on the brand teal.

| Foreground / background | Dark ratio | Light ratio | Result |
|---|---:|---:|---|
| primary text / background | 17.42:1 | 13.42:1 | AA |
| secondary text / background | 11.87:1 | 7.18:1 | AA |
| muted text / background | 7.92:1 | 5.06:1 | AA |
| accent text / background | 9.38:1 | 4.66:1 | AA |
| success text / background | 9.68:1 | 4.96:1 | AA |
| warning text / background | 10.46:1 | 5.73:1 | AA |
| danger text / background | 7.05:1 | 6.07:1 | AA |
| info text / background | 9.02:1 | 6.05:1 | AA |
| idle text / background | 7.36:1 | 4.63:1 | AA |
| navy text / brand teal fill | 7.23:1 | 7.23:1 | AA |
| border / background | 3.36:1 | 3.17:1 | non-text AA |
Card and pane text pairs also meet AA. Values use the same foreground tokens on dark surface #111C2E and light surface #FFFFFF.

| Foreground / surface | Dark ratio | Light ratio | Result |
|---|---:|---:|---|
| primary text | 15.90:1 | 14.42:1 | AA |
| secondary text | 10.83:1 | 7.71:1 | AA |
| muted text | 7.23:1 | 5.44:1 | AA |
| accent text | 8.56:1 | 5.01:1 | AA |
| success text | 8.83:1 | 5.33:1 | AA |
| warning text | 9.54:1 | 6.16:1 | AA |
| danger text | 6.43:1 | 6.52:1 | AA |
| info text | 8.23:1 | 6.50:1 | AA |
| idle text | 6.72:1 | 4.97:1 | AA |

Use token foregrounds on their corresponding canvas/panel colors; recompute contrast if a token is changed or a new pair is introduced. Disabled controls may be muted, but their label and reason remain perceivable.

## Typography

Web uses system sans stack `Inter, ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif`; code and IDs use `ui-monospace, SFMono-Regular, Consolas, monospace`. TUI uses terminal monospace only. Type scale: 12/16 metadata; 13/19 body; 14/21 emphasized body; 16/24 section title; 20/28 page title; 28/34 hero. Use weight 400 body, 500 labels, 600 titles. Avoid long all-caps labels.

## Layout and spacing

Use a 4 px base spacing scale: `4, 8, 12, 16, 20, 24, 32, 40, 48`. Web content gutters 24 px desktop and 16 px narrow; pane gaps 0 with 1 px dividers. TUI uses cell-based spacing, 1 blank cell inside borders, and no decorative whitespace that hides status or input. Radii: 4 px controls, 8 px cards, 12 px large dialogs; TUI uses square corners. Elevation: one subtle shadow on dialogs/menus only; rely on surface contrast and borders elsewhere.

## Components and states

Buttons: primary filled accent, secondary outlined, danger outlined until final destructive confirmation. Inputs: visible label, persistent value, placeholder only as hint. Chips: compact text plus optional status icon. Status dots: 8 px with adjacent label. Tabs: clear selected underline and keyboard focus. Sidebar rows: 32 px minimum, selected surface plus accent marker. Chat bubbles: user message aligned right, assistant response left; preserve readable line length. Tool-call rows: collapsed by default with tool, time and outcome; expanded output is sanitized and bounded. Dialogs: title, target/action summary, scope/expiry where applicable, cancel and explicit primary action. Toasts: short outcome plus next step, announced accessibly. Tables: aligned labels, sortable headers with state, empty and error rows. Progress: task/check state and verified counts, not model-invented percentages.

Every interactive element has default, hover, focus-visible, selected, disabled, loading, empty and error treatment where relevant; offline appears only on network-dependent Cloud/sync functions. TUI has equivalent focus/selection/disabled feedback without hover.

## TUI color and glyph fallbacks

Prefer truecolor, then ANSI-256 nearest color, then semantic 16-color foreground/background pairs. On `NO_COLOR` or non-TTY output, remove color and use explicit labels/brackets. Do not use color as the only signal. Box-drawing may be used when terminal support is present; fall back to ASCII `+ - | >` on legacy terminals. Preserve all layout and status text without Unicode glyph support. Keep text readable at 80x24 and allow horizontal detail to collapse before clipping controls.

| Token family | Truecolor | ANSI-256 | 16-color foreground |
|---|---|---:|---|
| background | `#0B1220` / `#F4F7FB` | 17 / 255 | black / white |
| surface | `#111C2E` / `#FFFFFF` | 17 / 15 | black / white |
| elevated | `#1B2A40` / `#FFFFFF` | 23 / 15 | bright-black / bright-white |
| border | `#596982` / `#7F8CA0` | 60 / 102 | bright-black / bright-black |
| text-primary | `#F4F7FB` / `#172A46` | 255 / 17 | bright-white / blue |
| text-secondary | `#C3CFDF` / `#44546A` | 188 / 60 | white / bright-black |
| text-muted | `#9AAAC0` / `#5B6B80` | 146 / 60 | bright-black / bright-black |
| accent | `#1ECFA5` / `#087E68` | 43 / 29 | bright-cyan / cyan |
| success, status-done | `#43D19E` / `#137A54` | 78 / 29 | bright-green / green |
| warning, status-working | `#F2B84B` / `#8A5600` | 221 / 130 | bright-yellow / yellow |
| danger, status-error | `#FF7272` / `#B4232F` | 203 / 160 | bright-red / red |
| info | `#79B8FF` / `#145FA8` | 111 / 25 | bright-blue / blue |
| status-idle | `#9AA3B2` / `#667085` | 146 / 60 | bright-black / bright-black |
| status-ready | `#1ECFA5` / `#087E68` | 43 / 29 | bright-cyan / cyan |

For 16-color terminals, use semantic foregrounds on the terminal's default background; do not force a dark background that can make light-terminal themes unreadable. For background/surface fills, use default background and visible borders when truecolor/256-color is unavailable.

## Iconography

Use a consistent, small set of familiar line icons: search, project/folder, worktree/branch, session/chat, agent, task/check, settings, history, memory, cloud/sync and warning/error. Icons support visible text and never replace labels. Prefer the supplied logo for brand identity; use ASCII symbols in TUI fallback mode, not emoji as status indicators. Use a check mark only for verified success, a spinner only for active work, and an exclamation only for a user-visible warning or error. Keep stroke and weight consistent within each surface.
## Motion and accessibility

Motion is brief (120–180 ms) and functional: focus, panel reveal, state change. No continuous decorative animation; respect reduced motion. Keyboard focus is always visible. Web targets WCAG 2.2 AA, semantic landmarks, logical heading order, accessible names, live announcements for state changes, keyboard-only operation, 200% zoom and reflow. TUI supports Tab/Shift+Tab, arrows, Enter, Escape, Ctrl+C cancellation when safe, and printable labels for screen readers; do not bind essential actions only to mouse or color. Confirm destructive actions with a readable target summary.

## Machine-readable tokens

```yaml
tokens:
  brand:
    navy: "#172A46"
    teal: "#1ECFA5"
  themes:
    dark:
      background: "#0B1220"
      surface: "#111C2E"
      elevated: "#1B2A40"
      border: "#596982"
      text_primary: "#F4F7FB"
      text_secondary: "#C3CFDF"
      text_muted: "#9AAAC0"
      accent: "#1ECFA5"
      success: "#43D19E"
      warning: "#F2B84B"
      danger: "#FF7272"
      info: "#79B8FF"
      status_working: "#F2B84B"
      status_done: "#43D19E"
      status_idle: "#9AA3B2"
      status_error: "#FF7272"
      status_ready: "#1ECFA5"
    light:
      background: "#F4F7FB"
      surface: "#FFFFFF"
      elevated: "#FFFFFF"
      border: "#7F8CA0"
      text_primary: "#172A46"
      text_secondary: "#44546A"
      text_muted: "#5B6B80"
      accent: "#087E68"
      success: "#137A54"
      warning: "#8A5600"
      danger: "#B4232F"
      info: "#145FA8"
      status_working: "#8A5600"
      status_done: "#137A54"
      status_idle: "#667085"
      status_error: "#B4232F"
      status_ready: "#087E68"
  tui_fallback:
    ansi256_dark: { background: 17, surface: 17, elevated: 23, border: 60, text_primary: 255, text_secondary: 188, text_muted: 146, accent: 43, success: 78, warning: 221, danger: 203, info: 111, status_working: 221, status_done: 78, status_idle: 146, status_error: 203, status_ready: 43 }
    ansi256_light: { background: 255, surface: 15, elevated: 15, border: 102, text_primary: 17, text_secondary: 60, text_muted: 60, accent: 29, success: 29, warning: 130, danger: 160, info: 25, status_working: 130, status_done: 29, status_idle: 60, status_error: 160, status_ready: 29 }
    ansi16_dark: { background: black, surface: black, elevated: bright_black, border: bright_black, text_primary: bright_white, text_secondary: white, text_muted: bright_black, accent: bright_cyan, success: bright_green, warning: bright_yellow, danger: bright_red, info: bright_blue, status_working: bright_yellow, status_done: bright_green, status_idle: bright_black, status_error: bright_red, status_ready: bright_cyan }
    ansi16_light: { background: white, surface: white, elevated: bright_white, border: bright_black, text_primary: blue, text_secondary: bright_black, text_muted: bright_black, accent: cyan, success: green, warning: yellow, danger: red, info: blue, status_working: yellow, status_done: green, status_idle: bright_black, status_error: red, status_ready: cyan }
  no_color: "Use default terminal colors and explicit words/icons; never encode status by color alone."
```

# TUI — Onboarding dashboard and Community/Pro

- **Surface:** TUI
- **Purpose:** Separate auto/manual/always launch radios from Community sem conta and optional Pro planejado
- **User goal:** Review the displayed information in Onboarding dashboard and Community/Pro and complete its main task or choose the next safe action.
- **Issue references:** #31,#33,#32,#19
- **Implementation status:** **planned**, based on inspected code on 2026-10-28; this is a design, not an implementation claim.

## Layout, region by region

Separate auto/manual/always launch radios from Community sem conta and optional Pro planejado. Arrows/Enter; Pro only opt-in; next summary. No invented prices/paywall.

## Wireframes

80x24 (80 columns by 24 rows):

```text
+------------------------------------------------------------------------------+
| Onboarding dashboard and Community/Pro                                       |
| Status: planned                                                              |
| Project: vitrine-api                                                         |
| Trust: granted | Run: none                                                   |
| Separate auto/manual/always launch radios from Community sem conta and optio |
|                                                                              |
| Enter confirm | Esc back                                                     |
| Ctrl+C cancel safely                                                         |
|                                                                              |
|                                                                              |
|                                                                              |
|                                                                              |
|                                                                              |
|                                                                              |
|                                                                              |
|                                                                              |
|                                                                              |
|                                                                              |
|                                                                              |
|                                                                              |
|                                                                              |
|                                                                              |
+------------------------------------------------------------------------------+
```

120x36 (120 columns by 36 rows):

```text
+----------------------------------------------------------------------------------------------------------------------+
| Onboarding dashboard and Community/Pro                                                                               |
| Status planned | Issues #31,#33,#32,#19                                                                              |
| Project vitrine-api | worktree feature/catalogo                                                                      |
| Separate auto/manual/always launch radios from Community sem conta and optional Pro planejado. Arrows/Enter; Pro onl |
| Run R-204 | Worker Codex gpt-6-luna | working                                                                        |
| Tokens 1.2k provider-reported | cost N/D                                                                             |
| Selected > Task T2 | attempt 1 | checks visible                                                                      |
|                                                                                                                      |
| Enter open | Esc back | / filter | ? help                                                                            |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
|                                                                                                                      |
+----------------------------------------------------------------------------------------------------------------------+
```

## Components and states

Components: Separate auto/manual/always launch radios from Community sem conta and optional Pro planejado. Arrows/Enter; Pro only opt-in; next summary. No invented prices/paywall.

- **Default:** show the representative content and current selection.
- **Hover/focus:** terminal has no hover; show visible keyboard focus without relying on color.
- **Selected:** use a text marker and semantic color plus a distinct label.
- **Disabled:** state why unavailable and never imply it ran.
- **Loading:** restrained spinner, “verificando…” and last-updated hint; allow safe cancellation.
- **Empty:** explain absence and offer a relevant next step, not synthetic records.
- **Error:** show plain-language cause, safe recovery and preserve input/completed work.
- **Offline:** keep local data visible and disable only network-dependent functions; otherwise not applicable.

## Sample data

Separate auto/manual/always launch radios from Community sem conta and optional Pro planejado. Arrows/Enter; Pro only opt-in; next summary. No invented prices/paywall. Sample content: Separate auto/manual/always launch radios from Community sem conta and optional Pro planejado. Arrows/Enter; Pro only opt-in; next summary. No invented prices/paywall. Use fictional project/session records; claude-sonnet-5-5 and gpt-6-luna are illustrative. Use relative local timestamps and label every metric provider-reported, measured, estimated or unavailable.

## Keyboard, transitions, accessibility, exclusions

Separate auto/manual/always launch radios from Community sem conta and optional Pro planejado. Arrows/Enter; Pro only opt-in; next summary. No invented prices/paywall. Follow [the TUI shared prompt](00-design-system-tui.md) and [DESIGN.md](../DESIGN.md): readable labels and glyphs, logical reading order, visible focus, NO_COLOR, reduced motion, and essential controls inside 80x24. Wide layouts add detail without changing command semantics.

**Not included:** Features and actions outside this screen scope; no secrets, hidden reasoning, or fabricated behavior.

## Copy-paste prompt

```PROMPT
Read docs/design/DESIGN.md and docs/design/prompts/00-design-system-tui.md first, then draw the GasaCLI Onboarding dashboard and Community/Pro for a local-first coding-agent orchestrator. This is currently planned; label planned and unavailable actions honestly. Follow this screen plan exactly: Separate auto/manual/always launch radios from Community sem conta and optional Pro planejado. Arrows/Enter; Pro only opt-in; next summary. No invented prices/paywall. Keep project identity, focus, state and usable input visible. Render matching versions at exactly 80x24 and 120x36, use ASCII borders where needed, and never clip primary controls.

Use these Brazilian Portuguese sample strings and data: Separate auto/manual/always launch radios from Community sem conta and optional Pro planejado. Arrows/Enter; Pro only opt-in; next summary. No invented prices/paywall. Optional adjacent-session sample, show only when relevant: project vitrine-api; Worker Codex using gpt-6-luna; activity há 8 min; 1.2k tokens provider-reported; API-equivalent cost N/D. First-run and empty screens must not surface session metrics. Include the listed components and show default, selected, disabled, loading, empty, error and offline states. Terminals do not have hover, so make keyboard focus obvious. Display the documented shortcuts and transitions. Preserve Run, Task, Dispatch and Worker vocabulary where useful. Use relative timestamps and label usage by provenance. Never invent a successful Run, agent output, review, approval, progress, token usage or cost; do not expose credentials or private model reasoning.

Match the referenced design system in dark/light themes and NO_COLOR/16-color terminals. Status must remain understandable without color. Use only actions described for this screen and explain anything unavailable. Exclude unrelated capabilities. Make the result look like a credible terminal product interface, not marketing art, while keeping both sizes equally usable.
```

# TUI — Onboarding language

- **Surface:** TUI
- **Purpose:** Step 1/6
- **User goal:** Review the displayed information in Onboarding language and complete its main task or choose the next safe action.
- **Issue references:** #31
- **Implementation status:** **planned**, based on inspected code on 2026-10-28; this is a design, not an implementation claim.

## Layout, region by region
Character-cell composition: Radio rows Português (Brasil) recommended and English, helper and back/continue/skip. Keep primary context, selection and action visible at 80x24; reveal secondary detail at 120x36. Never add planned/status banners to the mockup.

## Wireframes

80x24 (80 columns by 24 rows):

```text
+------------------------------------------------------------------------------+
| Onboarding language                                                          |
| Project: vitrine-api                                                         |
| Trust: granted | Run: none                                                   |
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
| Onboarding language                                                                                                  |
| Project vitrine-api | worktree feature/catalogo                                                                      |
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
Present only controls needed for the task. Use explicit labels for default, selected, disabled, loading, empty and error states; show offline only on network-dependent actions. Keep focus visible. TUI has no hover; show visible keyboard focus.

## Sample data
Use only the fictional sample values specified in the screen's composition and wireframe. Keep UI copy in pt-BR; do not add users, secrets, prices, versions, metrics or agent activity. Unknown values stay unavailable, never zero.

## Keyboard, transitions, accessibility, exclusions
Follow the screen's stated shortcuts and transitions. Keep labels understandable without color and preserve safe cancellation and confirmation behavior. Privacy wording: "GasaCLI does not collect telemetry; each agent follows its provider's policy". Never claim no bytes leave the machine, isolation is confirmed, or code has zero telemetry. Exclude secrets and private model reasoning.

## Copy-paste prompt

```PROMPT
Design the GasaCLI — Onboarding language screen for a local-first coding-agent workspace. The user's goal is: Review the displayed information in Onboarding language and complete its main task or choose the next safe action.

Screen composition: Radio rows Português (Brasil) recommended and English, helper and back/continue/skip.

Render only a real character-cell terminal: monospace grid, square borders, box-drawing/ASCII, ANSI-style colors, visible block cursor/selection and keyboard hint bar in a plain terminal frame. No web chrome, sidebar, app bar, Material icons, rounded cards, blur, glow or emoji. Use 120x36, or 80x24 where specified. Exact documented hex values only, especially navy #172A46 and teal #1ECFA5. Draw a compact G+chevron box-drawing mark of at most 7 lines; no block-letter GASACLI banner. Use only glossary names. Onboarding has exactly 6 numbered steps; trust confirmation is an unnumbered interstitial. No external fonts/CDN. NO status banners in the mockup.

Use only the sample values from the screen specification: Sample “Você pode alterar depois”.. Do not extend them with new facts. Use only glossary names: GasaCLI, Claude Code, Codex CLI, Gemini CLI, Cursor CLI, OpenCode, Orca, Kimi, claude-sonnet-5-5 and gpt-6-luna. Render UI copy in pt-BR. Use exact color tokens from docs/design/DESIGN.md, especially navy #172A46 and teal #1ECFA5. Status must have a readable label as well as color. Never invent product facts, names, versions, RFCs, sockets, prices, metrics, progress, costs, approvals or successful Runs. Use Run, Task, Dispatch and Worker accurately. Unknown usage is unavailable, not zero. Never show credentials, raw environment values or private model reasoning. Privacy copy: "GasaCLI does not collect telemetry; each agent follows its provider's policy". Never claim no bytes leave the machine, isolation is confirmed, or code has zero telemetry.

Follow the screen's listed actions and shortcuts. Show default, focus/selected, disabled, loading, empty and error states; show offline only for network-dependent functions. TUI has no hover; make focus unmistakable, keep primary controls visible at both sizes, and preserve meaning with NO_COLOR/ANSI fallbacks. Keep the design focused on its stated purpose. Do not add marketing copy, invented data or unrelated controls.
```

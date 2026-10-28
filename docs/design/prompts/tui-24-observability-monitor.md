# TUI — Observability monitor

- **Surface:** TUI
- **Purpose:** Run/time filters, RAM/CPU/token/cost cards, parent/child process chart and coverage labels
- **User goal:** Review the displayed information in Observability monitor and complete its main task or choose the next safe action.
- **Issue references:** #21,#24
- **Implementation status:** **planned**, based on inspected code on 2026-10-28; this is a design, not an implementation claim.

## Layout, region by region

Run/time filters, RAM/CPU/token/cost cards, parent/child process chart and coverage labels. Sample GasaCLI 38MB measured, Codex 420MB measured, 18,420 tokens reported, cost N/D, coverage 1/2. Tab filters, arrows, Enter Run. No unknown=zero.

## Wireframes

80x24 (80 columns by 24 rows):

```text
+------------------------------------------------------------------------------+
| Observability monitor                                                        |
| Status: planned                                                              |
| Project: vitrine-api                                                         |
| Trust: granted | Run: none                                                   |
| Run/time filters, RAM/CPU/token/cost cards, parent/child process chart and c |
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
| Observability monitor                                                                                                |
| Status planned | Issues #21,#24                                                                                      |
| Project vitrine-api | worktree feature/catalogo                                                                      |
| Run/time filters, RAM/CPU/token/cost cards, parent/child process chart and coverage labels. Sample GasaCLI 38MB meas |
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

Components: Run/time filters, RAM/CPU/token/cost cards, parent/child process chart and coverage labels. Sample GasaCLI 38MB measured, Codex 420MB measured, 18,420 tokens reported, cost N/D, coverage 1/2. Tab filters, arrows, Enter Run. No unknown=zero.

- **Default:** show the representative content and current selection.
- **Hover/focus:** terminal has no hover; show visible keyboard focus without relying on color.
- **Selected:** use a text marker and semantic color plus a distinct label.
- **Disabled:** state why unavailable and never imply it ran.
- **Loading:** restrained spinner, “verificando…” and last-updated hint; allow safe cancellation.
- **Empty:** explain absence and offer a relevant next step, not synthetic records.
- **Error:** show plain-language cause, safe recovery and preserve input/completed work.
- **Offline:** keep local data visible and disable only network-dependent functions; otherwise not applicable.

## Sample data

Run/time filters, RAM/CPU/token/cost cards, parent/child process chart and coverage labels. Sample GasaCLI 38MB measured, Codex 420MB measured, 18,420 tokens reported, cost N/D, coverage 1/2. Tab filters, arrows, Enter Run. No unknown=zero. Sample content: Run/time filters, RAM/CPU/token/cost cards, parent/child process chart and coverage labels. Sample GasaCLI 38MB measured, Codex 420MB measured, 18,420 tokens reported, cost N/D, coverage 1/2. Tab filters, arrows, Enter Run. No unknown=zero. Use fictional project/session records; claude-sonnet-5-5 and gpt-6-luna are illustrative. Use relative local timestamps and label every metric provider-reported, measured, estimated or unavailable.

## Keyboard, transitions, accessibility, exclusions

Run/time filters, RAM/CPU/token/cost cards, parent/child process chart and coverage labels. Sample GasaCLI 38MB measured, Codex 420MB measured, 18,420 tokens reported, cost N/D, coverage 1/2. Tab filters, arrows, Enter Run. No unknown=zero. Follow [the TUI shared prompt](00-design-system-tui.md) and [DESIGN.md](../DESIGN.md): readable labels and glyphs, logical reading order, visible focus, NO_COLOR, reduced motion, and essential controls inside 80x24. Wide layouts add detail without changing command semantics.

**Not included:** Features and actions outside this screen scope; no secrets, hidden reasoning, or fabricated behavior.

## Copy-paste prompt

```PROMPT
Read docs/design/DESIGN.md and docs/design/prompts/00-design-system-tui.md first, then draw the GasaCLI Observability monitor for a local-first coding-agent orchestrator. This is currently planned; label planned and unavailable actions honestly. Follow this screen plan exactly: Run/time filters, RAM/CPU/token/cost cards, parent/child process chart and coverage labels. Sample GasaCLI 38MB measured, Codex 420MB measured, 18,420 tokens reported, cost N/D, coverage 1/2. Tab filters, arrows, Enter Run. No unknown=zero. Keep project identity, focus, state and usable input visible. Render matching versions at exactly 80x24 and 120x36, use ASCII borders where needed, and never clip primary controls.

Use these Brazilian Portuguese sample strings and data: Run/time filters, RAM/CPU/token/cost cards, parent/child process chart and coverage labels. Sample GasaCLI 38MB measured, Codex 420MB measured, 18,420 tokens reported, cost N/D, coverage 1/2. Tab filters, arrows, Enter Run. No unknown=zero. Optional adjacent-session sample, show only when relevant: project vitrine-api; Worker Codex using gpt-6-luna; activity há 8 min; 1.2k tokens provider-reported; API-equivalent cost N/D. First-run and empty screens must not surface session metrics. Include the listed components and show default, selected, disabled, loading, empty, error and offline states. Terminals do not have hover, so make keyboard focus obvious. Display the documented shortcuts and transitions. Preserve Run, Task, Dispatch and Worker vocabulary where useful. Use relative timestamps and label usage by provenance. Never invent a successful Run, agent output, review, approval, progress, token usage or cost; do not expose credentials or private model reasoning.

Match the referenced design system in dark/light themes and NO_COLOR/16-color terminals. Status must remain understandable without color. Use only actions described for this screen and explain anything unavailable. Exclude unrelated capabilities. Make the result look like a credible terminal product interface, not marketing art, while keeping both sizes equally usable.
```

# Web — Per-project analytics drilldown

- **Surface:** Web
- **Purpose:** Project scope and same global filters; Runs/Tasks/agents breakdown, quality/retry and known-usage coverage
- **User goal:** Review the displayed information in Per-project analytics drilldown and complete its main task or choose the next safe action.
- **Issue references:** #24,#23
- **Implementation status:** **planned**, based on the inspected repository on 2026-10-28; this design is not an implementation claim.

## Layout, region by region
Center-pane composition: Project scope and same global filters; Runs/Tasks/agents breakdown, quality/retry and known-usage coverage. Keep persistent left navigation and Changes/Checks/Info panes. At medium widths move right details into a drawer; at narrow widths use one pane with clear back navigation.

## Pane sketch

```text
+-----------------------+--------------------------------+---------------------+
| Search / Projects     | Tabs: Workspace / Per-project  | Changes Checks Info |
|   feature/catalogo    | Project scope and same glob | planned            |
|   Claude • 8 min      | Content and primary action     | Tokens: N/D         |
+-----------------------+--------------------------------+---------------------+
```

## Components and states
Present only controls needed for the task. Use explicit labels for default, selected, disabled, loading, empty and error states; show offline only on network-dependent actions. Keep focus visible. Web hover is supplementary and never replaces keyboard focus.

## Sample data
Use only the fictional sample values specified in the screen's composition and wireframe. Keep UI copy in pt-BR; do not add users, secrets, prices, versions, metrics or agent activity. Unknown values stay unavailable, never zero.

## Keyboard, transitions, accessibility and exclusions
Follow the screen's stated shortcuts and transitions. Keep labels understandable without color and preserve safe cancellation and confirmation behavior. Privacy wording: "GasaCLI does not collect telemetry; each agent follows its provider's policy". Never claim no bytes leave the machine, isolation is confirmed, or code has zero telemetry. Exclude secrets and private model reasoning.

## Copy-paste prompt

```PROMPT
Design the GasaCLI — Per-project analytics drilldown screen for a local-first coding-agent workspace. The user's goal is: Review the displayed information in Per-project analytics drilldown and complete its main task or choose the next safe action.

Screen composition: Project scope and same global filters; Runs/Tasks/agents breakdown, quality/retry and known-usage coverage.

Preserve the Orca-like three-pane shell with responsive drawers and one-pane mobile layout. Use supplied docs/img/logo.png as the canonical G+chevron mark. Exact documented hex values only, especially navy #172A46 and teal #1ECFA5. No external fonts, icon fonts, CDN or status banners in the mockup.

Use only the sample values from the screen specification: Sample vitrine-api 7d, 4 Runs, 3 passed reviews, tokens reported in 2/4.. Do not extend them with new facts. Use only glossary names: GasaCLI, Claude Code, Codex CLI, Gemini CLI, Cursor CLI, OpenCode, Orca, Kimi, claude-sonnet-5-5 and gpt-6-luna. Render UI copy in pt-BR. Use exact color tokens from docs/design/DESIGN.md, especially navy #172A46 and teal #1ECFA5. Status must have a readable label as well as color. Never invent product facts, names, versions, RFCs, sockets, prices, metrics, progress, costs, approvals or successful Runs. Use Run, Task, Dispatch and Worker accurately. Unknown usage is unavailable, not zero. Never show credentials, raw environment values or private model reasoning. Privacy copy: "GasaCLI does not collect telemetry; each agent follows its provider's policy". Never claim no bytes leave the machine, isolation is confirmed, or code has zero telemetry.

Follow the screen's listed actions and shortcuts. Show default, focus/selected, disabled, loading, empty and error states; show offline only for network-dependent functions. Use semantic landmarks and accessible names, visible keyboard focus, reduced motion, 200% zoom and responsive reflow. Keep the design focused on its stated purpose. Do not add marketing copy, invented data or unrelated controls.
```

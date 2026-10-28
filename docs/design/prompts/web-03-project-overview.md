# Web — Project overview

- **Surface:** Web
- **Purpose:** Within three-pane shell, project header/path/trust and branch; summary cards for latest Run, active agents, gates, recent activity; right Info
- **User goal:** Review the displayed information in Project overview and complete its main task or choose the next safe action.
- **Issue references:** #23,#24,#27
- **Implementation status:** **partial**, based on the inspected repository on 2026-10-28; this design is not an implementation claim.

## Layout, region by region
Center-pane composition: Within three-pane shell, project header/path/trust and branch; summary cards for latest Run, active agents, gates, recent activity; right Info. Keep persistent left navigation and Changes/Checks/Info panes. At medium widths move right details into a drawer; at narrow widths use one pane with clear back navigation.

## Pane sketch

```text
+-----------------------+--------------------------------+---------------------+
| Search / Projects     | Tabs: Workspace / Project over | Changes Checks Info |
|   feature/catalogo    | Within three-pane shell, pr | partial            |
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
Design the GasaCLI — Project overview screen for a local-first coding-agent workspace. The user's goal is: Review the displayed information in Project overview and complete its main task or choose the next safe action.

Screen composition: Within three-pane shell, project header/path/trust and branch; summary cards for latest Run, active agents, gates, recent activity; right Info.

Preserve the Orca-like three-pane shell with responsive drawers and one-pane mobile layout. Use supplied docs/img/logo.png as the canonical G+chevron mark. Exact documented hex values only, especially navy #172A46 and teal #1ECFA5. No external fonts, icon fonts, CDN or status banners in the mockup.

Use only the sample values from the screen specification: Sample vitrine-api, feature/catalogo, no active Run, two detected sessions, tokens N/D.. Do not extend them with new facts. Use only glossary names: GasaCLI, Claude Code, Codex CLI, Gemini CLI, Cursor CLI, OpenCode, Orca, Kimi, claude-sonnet-5-5 and gpt-6-luna. Render UI copy in pt-BR. Use exact color tokens from docs/design/DESIGN.md, especially navy #172A46 and teal #1ECFA5. Status must have a readable label as well as color. Never invent product facts, names, versions, RFCs, sockets, prices, metrics, progress, costs, approvals or successful Runs. Use Run, Task, Dispatch and Worker accurately. Unknown usage is unavailable, not zero. Never show credentials, raw environment values or private model reasoning. Privacy copy: "GasaCLI does not collect telemetry; each agent follows its provider's policy". Never claim no bytes leave the machine, isolation is confirmed, or code has zero telemetry.

Follow the screen's listed actions and shortcuts. Show default, focus/selected, disabled, loading, empty and error states; show offline only for network-dependent functions. Use semantic landmarks and accessible names, visible keyboard focus, reduced motion, 200% zoom and responsive reflow. Keep the design focused on its stated purpose. Do not add marketing copy, invented data or unrelated controls.
```

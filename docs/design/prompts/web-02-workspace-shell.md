# Web — Workspace shell: sidebar and tabs

- **Surface:** Web
- **Purpose:** Orca-like desktop three-pane shell
- **User goal:** Review the displayed information in Workspace shell: sidebar and tabs and complete its main task or choose the next safe action.
- **Issue references:** #22,#23
- **Implementation status:** **partial**, based on the inspected repository on 2026-10-28; this design is not an implementation claim.

## Layout, region by region

Use the shared desktop shell: left sidebar (search, navigation, projects > worktrees > agent sessions with status dot, model and relative age); center pane (workspace tabs, screen title and main content); collapsible right pane with Changes, Checks and Info. Screen-specific arrangement: Orca-like desktop three-pane shell. Left search/nav/projects/worktrees/sessions with status dot/model/age; center workspace tabs/content; right collapsible Changes/Checks/Info. Sample projects vitrine-api and loja-aurora; Claude/gpt-6-luna; 8 min. Keyboard search, nav, tabs; next project/session. Existing shell is partial and has read-only session discovery. On medium widths collapse right details to a drawer; on mobile use a single pane with back navigation.

## Pane sketch

```text
+-----------------------+--------------------------------+---------------------+
| Search / Projects     | Tabs: Workspace / Workspace sh | Changes Checks Info |
| > vitrine-api         | Project: vitrine-api           | Run: R-204          |
|   feature/catalogo    | Orca-like desktop three-pan | partial            |
|   Claude • 8 min      | Content and primary action     | Tokens: N/D         |
+-----------------------+--------------------------------+---------------------+
```

## Components and states

Components: Orca-like desktop three-pane shell. Left search/nav/projects/worktrees/sessions with status dot/model/age; center workspace tabs/content; right collapsible Changes/Checks/Info. Sample projects vitrine-api and loja-aurora; Claude/gpt-6-luna; 8 min. Keyboard search, nav, tabs; next project/session. Existing shell is partial and has read-only session discovery.

- **Default:** screen content in the selected project/session context.
- **Hover/focus:** visible focus ring; hover changes surface only and never hides labels.
- **Selected:** row/tab/card uses shape or weight plus semantic status label.
- **Disabled:** explain whether unavailable, read-only, planned or blocked and why.
- **Loading:** skeleton with text status and bounded progress; no endless spinner.
- **Empty:** explain absence and offer a relevant action; do not create synthetic user records.
- **Error:** plain cause, retry/recovery and retained-data explanation.
- **Offline:** preserve local content; disable only network-backed Cloud/plugin actions and label sync state.

## Sample data

Orca-like desktop three-pane shell. Left search/nav/projects/worktrees/sessions with status dot/model/age; center workspace tabs/content; right collapsible Changes/Checks/Info. Sample projects vitrine-api and loja-aurora; Claude/gpt-6-luna; 8 min. Keyboard search, nav, tabs; next project/session. Existing shell is partial and has read-only session discovery. UI labels are pt-BR; examples are fictional and contain no real email, credentials or activation code. Use relative times and model labels such as claude-sonnet-5-5 and gpt-6-luna only as illustrative data. Label metrics measured, provider-reported, estimated or unavailable. Optional adjacent-session sample, show only when relevant: project vitrine-api; Worker Codex using gpt-6-luna; activity há 8 min; 1.2k tokens provider-reported; API-equivalent cost N/D. First-run and empty screens must not surface session metrics.

## Keyboard, transitions, accessibility and exclusions

Orca-like desktop three-pane shell. Left search/nav/projects/worktrees/sessions with status dot/model/age; center workspace tabs/content; right collapsible Changes/Checks/Info. Sample projects vitrine-api and loja-aurora; Claude/gpt-6-luna; 8 min. Keyboard search, nav, tabs; next project/session. Existing shell is partial and has read-only session discovery. Follow [the Web shared design prompt](00-design-system-web.md) and [DESIGN.md](../DESIGN.md): semantic landmarks, keyboard-only access, focus-visible, reduced motion, 200% zoom, reflow, accessible live states and mobile drawer behavior. Keep project/session identity visible across navigation.

**Not included:** Features and actions outside this screen scope; no secrets, hidden reasoning, or fabricated behavior.

## Copy-paste prompt

```PROMPT
Read docs/design/DESIGN.md and docs/design/prompts/00-design-system-web.md first, then design the GasaCLI Workspace shell: sidebar and tabs screen for a local-first coding-agent workspace. It is currently partial in the inspected product; make planned, disabled, read-only and unavailable behavior explicit. Use the Orca-like three-pane frame: searchable navigation and projects/worktrees/sessions at left with status, model and age; tabs and the main screen in the center; collapsible Changes, Checks and Info at right. Use this screen-specific content: Orca-like desktop three-pane shell. Left search/nav/projects/worktrees/sessions with status dot/model/age; center workspace tabs/content; right collapsible Changes/Checks/Info. Sample projects vitrine-api and loja-aurora; Claude/gpt-6-luna; 8 min. Keyboard search, nav, tabs; next project/session. Existing shell is partial and has read-only session discovery. Keep the active project/session visible; collapse the right pane to a drawer at medium width and use a single-pane back-navigation layout on mobile.

Render concise pt-BR UI strings and believable fictional data from the screen plan. Optional adjacent-session sample, show only when relevant: project vitrine-api; Worker Codex using gpt-6-luna; activity há 8 min; 1.2k tokens provider-reported; API-equivalent cost N/D. First-run and empty screens must not surface session metrics. Include all listed components, plus default, hover/focus, selected, disabled, loading, empty, error and offline states. Make keyboard focus visible, use semantic status words with dots, and show an understandable reason for disabled actions. Use relative times; every metric needs measured, provider-reported, estimated or unavailable provenance. Unknown usage is not zero. Never show a real email, secret, activation code, credential, hidden model reasoning, fake successful Run or fabricated payment/Cloud capability.

Follow the referenced design system for color, typography, components, responsive behavior and WCAG AA. Provide keyboard access, reduced motion and accessible live announcements. Exclude unrelated features and keep this screen focused: Orca-like desktop three-pane shell. Left search/nav/projects/worktrees/sessions with status dot/model/age; center workspace tabs/content; right collapsible Changes/Checks/Info. Sample projects vitrine-api and loja-aurora; Claude/gpt-6-luna; 8 min. Keyboard search, nav, tabs; next project/session. Existing shell is partial and has read-only session discovery. Present a credible working product screen rather than a marketing page.
```

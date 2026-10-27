# ADR-0012 — Global and per-project analytics

Status: Accepted UX and data-model requirement (2026-10-27).

## Views
- Global overview: aggregate all trusted projects by date range, agent, provider, model, goal, status and session.
- Project analytics: same metrics filtered by project ID, with task/run drill-down and per-agent breakdown.
- Sidebar navigation shows active projects and global Overview.

## Metrics
Reported input/output/cache tokens, API-equivalent estimated cost, run duration, goal completion, retries, test/build/review outcomes, process RAM/CPU and agent activity. Distinguish measured, provider-reported, estimated and unavailable values. Never imply API-equivalent estimates are actual subscription charges.

## Data model
Every run belongs to a stable project ID, based on an explicitly registered project path/repository identity; tasks, dispatches, agent events, usage samples and quality gates reference run/task/agent IDs. Store monetary values as precise decimal or integer micros with currency, pricing source, effective date and model ID. Avoid floating-point accumulation. Token counts may be nullable when unavailable.

## Aggregation
Use SQLite indexed queries and precomputed summaries only when necessary. Sum only known usage; show coverage percentage for partial reporting, rather than treating unknown values as zero. Support filters for 24h, 7d, 30d and custom intervals, plus all-time.

## Example questions
- What is the API-equivalent spend across all projects this month?
- How many tokens did Codex report in one project?
- Which agent had the fewest retries for comparable task classes?
- Which runs are blocked and why?
- What share of executions passed independent review?

## Privacy and performance
Metrics stay local by default. Cloud sync of analytics is optional and opt-in. Do not persist raw secrets or private prompts in metrics. Bound process sampling and event retention. Never double-count child processes or token events when aggregating.

## Acceptance
Global totals equal sum of included project totals for the same filters and reporting coverage. Project isolation is enforced, unknown metrics are identified, and drill-down reaches the underlying run evidence.

# GasaCLI — Architecture (draft)

## Principles
Local-first memory; modular adapters; deterministic policy enforcement; observable decisions; minimal context injection; explicit approvals; recoverable execution.

## Request lifecycle
1. CLI parses command and resolves active project.
2. Memory engine reads project metadata, then retrieves relevant Markdown passages from a local incremental index (SQLite FTS5).
3. Decision engine classifies intent, complexity and risk; produces a structured plan with provenance.
4. Policy layer validates permissions, allowed tools, data boundaries and budget.
5. Model router chooses eligible provider/model per task based on capability, cost and fallback rules.
6. Execution adapter delegates to Orca or another backend; persists run state, checkpoints and events.
7. Quality gate evaluates tests/build/review; memory writer proposes updates for explicit approval.

## Boundaries
- `apps/cli`: terminal UX and configuration.
- `packages/core`: task contracts, policies, event types, persistence interfaces.
- `packages/decision-engine`: planning and risk classification.
- `packages/memory`: Obsidian Markdown ingestion, retrieval and source attribution.
- `packages/model-router`: provider selection and budget accounting.
- `packages/orchestrator`: executor adapter interfaces, Orca implementation.
- `packages/providers`: model API adapters.
- `packages/skills`: optional audited skills.

## Memory tiers
L1: active project context and cached decisions.
L2: SQLite FTS5 index with paths, timestamps, chunk hashes.
L3: original Markdown vault, read on demand.
Optional semantic search follows measured retrieval benchmarks.

## Data and safety
Never modify original notes during indexing. Vault writes require preview and approval. Store run state separately from user knowledge. Guard against prompt injection from Markdown, command output and third-party skills. Enforce budget and permissions in code rather than prompts. Avoid blanket shell access by default.

## Open questions
Confirm JEV's existing implementation, Ponytail 5 and Caveman repositories, Orca integration API, and target OS/platform matrix.

# ADR-0007 — Ultra-lightweight thin orchestrator

Status: Accepted as performance direction; numeric budgets are provisional until measured.

## Objective
GasaCLI must be exceptionally lightweight, comparable in intent to lightweight coding-agent launchers. It orchestrates installed coding CLIs, not local LLM inference. CPU/RAM of child processes must be reported separately from the GasaCLI process.

## Provisional budgets (release builds, baseline hardware to be defined)
- CLI executable target: <=25 MB.
- Idle resident memory target: <=40 MB.
- Warm startup target: <=300 ms.
- Idle CPU target: approximately 0%.
These are engineering targets, not observed benchmarks. Cross-platform and cold-start results may differ.

## Design
- Go-first native executable; avoid mandatory Node, Python, Redis, Celery, Docker or background daemon.
- SQLite embedded, WAL, bounded connections and incremental writes.
- Spawn Claude Code, Codex or other installed CLIs only on demand.
- Supervise subprocesses via context cancellation, process groups where supported, streaming stdout/stderr and bounded buffers.
- Event-driven TUI, no busy polling or permanent model-based coordination.
- Persist plan and run state; invoke an orchestration model only for decisions that need it.
- Lazy-load optional plugins, brain index and provider discovery; cache version/capability probes with invalidation.
- Bounded parallelism and backpressure to prevent process storms.
- Do not hold complete transcripts, large diffs or vaults in memory.
- Rust modules only if profiling proves a significant benefit and does not add unacceptable startup/packaging overhead.
- No cloud dependency for core functionality.

## Benchmarks and instrumentation
Build release binaries on Windows/Linux/macOS; measure binary size, cold/warm startup, RSS idle, CPU idle, peak orchestrator-only RSS, CLI discovery latency, first token latency, subprocess cleanup, 1/2/4 concurrent workers, resume and cancellation. Report process-tree totals separately.

## Acceptance
Automated performance regression tests with documented environment and thresholds; publish benchmark artifacts before release. If targets are missed, diagnose rather than quietly relaxing them.

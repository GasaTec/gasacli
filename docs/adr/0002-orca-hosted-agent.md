# ADR-0002: GasaCLI as an Orca-hosted conversational agent

Status: Proposed — requires verification of Orca agent-provider extension points.

## Goal
Allow GasaCLI to run independently as `gasa` and optionally appear in Orca's agent picker alongside coding CLIs such as Claude Code and Codex. GasaCLI is an orchestration runtime, not an LLM; it routes tasks to installed, authenticated coding CLIs.

## Proposed UX
```sh
gasa integration install orca
gasa integration doctor orca
gasa agent serve --transport stdio
```
Commands are proposed, not implemented. Do not claim Orca supports a third-party agent installer until its integration contract is verified.

## Integration contract
- Host adapter maps Orca conversation/session events to GasaCLI's own session model.
- Structured request/response envelopes, streamed progress, cancellation, errors and resumability.
- Surface approval prompts and permission decisions to the Orca host.
- Persist session state separately from downstream agent state.
- Pass working directory, project trust and host policy restrictions to child agents.
- Report child usage where available; do not claim exact token savings without measurements.
- Avoid recursion: when hosted by Orca, prevent dispatching to the same Orca host or nested GasaCLI in a cycle.
- Never silently install plugins or bypass provider subscriptions and limits.

## Research and delivery plan
1. Inspect current Orca source and documentation for agent registry, CLI adapters, extension/plugin APIs and subprocess protocols.
2. If external agent registration is supported, implement a standalone Orca adapter and installer using the documented interface.
3. If unsupported, prototype a compatible wrapper only where permitted, clearly mark limitations, and propose an upstream Orca issue/PR.
4. Test multi-turn conversation, streamed output, cancellation, approvals, error propagation, working directory and resume.
5. Maintain a clean standalone mode without an Orca dependency.

## Relation to ADR-0001
ADR-0001 defines GasaCLI's standalone local-first orchestration. This ADR adds an optional *inbound* Orca-hosted agent adapter; ADR-0001's optional Orca execution adapter is an *outbound* integration. These are separate directions and must not be confused.

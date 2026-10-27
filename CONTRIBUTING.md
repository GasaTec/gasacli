# Contributing to GasaCLI

Thanks for helping build an accessible, local-first agent orchestrator.

## Before contributing
- Read [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) and [SECURITY.md](SECURITY.md).
- Search existing issues and discussions before proposing a feature.
- For larger changes, open an issue describing the problem, design and acceptance criteria.
- Never include secrets, private vault content, real user conversations or credentials in issues or tests.

## Development workflow
1. Fork the repository or create a feature branch if you have write access.
2. Work on one scoped issue; name branches `feat/<issue>-short-name` or `fix/<issue>-short-name`.
3. Include tests for behavior changes and update documentation.
4. Run the repository's lint/typecheck/test/build scripts once those scripts are implemented. Do not claim they pass until executed.
5. Open a PR referencing the issue with summary, verification evidence, screenshots for TUI changes and any security implications.
6. Wait for review; maintainers may request revisions.

## Design principles
- Local-first, no mandatory cloud account or extra model API key.
- Provider-neutral CLI adapters and clear capability detection.
- No false DONE: verify acceptance, tests/build and independent review when required.
- Small, auditable prompts and bounded retries; measure cost rather than promising savings.
- Treat model output, repository files and Markdown notes as untrusted.
- Explicit approval for destructive operations and plugin/skill installation.
- Never overwrite a user's customized prompts, config or vault.

## Contributions
Code, docs, reproducible bugs, test cases, adapters, accessibility improvements, translations and reviewed skills are welcome. Keep PRs focused. Automated/AI-assisted contributions are welcome, but the submitter is responsible for reviewing the output and testing it.

## Licensing
By submitting a contribution, you agree that it will be distributed under this repository's MIT License. Only submit material you have the right to license. No contributor license agreement is currently required.

## Community
Be constructive and respectful. Use GitHub Issues for actionable work and Discussions (when enabled) for questions and proposals.

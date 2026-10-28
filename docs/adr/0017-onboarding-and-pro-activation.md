# ADR-0017 — First-run onboarding and optional Pro activation

Status: Proposed product contract, 2026-10-27.

## First run
1. Install signed/checksummed GasaCLI binary with embedded dashboard.
2. Run `gasa`; first-run onboarding checks CLI versions and supported readiness for Claude Code, Codex, Gemini, Cursor, OpenCode and others. Do not read tokens or infer subscription entitlements.
3. Show missing agents with official install guidance; never silently install third-party executables.
4. Ask project trust and permission profile; offer fixed routing setup and optional default agent assignments.
5. Present Community (free, no account) and Pro (optional managed services). Never gate local /goal, native orchestration, review, prompts, plugins, local backup or local SQLite behind Pro.
6. Pro: authenticate via browser using OAuth PKCE/device flow or redeem one-time activation code, then bind subscription entitlement to account and authorized device(s).
7. Launch TUI and local dashboard on preferred 127.0.0.1:3199 with safe fallback. Onboarding can be reopened with `/setup` or `gasa setup`.

## Pro entitlement
An activation code is a short-lived, one-time redemption credential, not a permanent reusable API key. Server atomically redeems it, records redemption status, binds to account and creates subscription entitlement. Client stores refresh credentials in OS vault, not YAML/SQLite plaintext. Device enrollment is revocable; entitlement checked periodically with bounded offline grace, without disabling free local features when Pro expires. Do not use hardware fingerprints as sole identity.

## Commercial architecture
Marketing landing page, checkout, payment webhook handling, subscription billing, license issuance and customer portal may live in a separate project/service. InfinitPay is an unverified candidate; see [ADR-0020](0020-cloud-identity-e2ee-and-pro-activation.md) and [Cloud design](../08-cloud-sync-e-monetizacao.md) for required primary-documentation and sandbox verification. Payment confirmation must come from authenticated verified server-to-server validation, not browser redirect alone. Ensure idempotency, anti-replay, code expiration, rate limits, audit log, refunds/cancellation and entitlement revocation.

## Trust boundaries
Open-source GasaCLI repository contains Community core and optional Cloud client/protocol/server as previously agreed. Separate commercial LP/billing service may be independently deployed, but do not claim MIT-licensed code in the repo is proprietary. Cloud stores account/subscription metadata separately from encrypted sync payloads.

## UX and commands (proposed)
`/setup`, `/plan`, `/cloud login`, `/cloud activate`, `/cloud status`, `/cloud logout`, `gasa doctor`.
Never pass activation codes or auth tokens in CLI arguments, URLs or logs; enter interactively.

## Acceptance
Community setup works fully offline from Gasatec services. Pro can be activated on an authorized device, reauthenticated and revoked; expired Pro leaves local sessions intact. Invalid/reused codes are rejected without leaking account information. Checkout cannot mint entitlements before verified payment.

## Amendment 2026-10-27 — consent-based installs
Onboarding may OFFER to install missing agents and the optional Obsidian app after the user explicitly selects them and confirms a plan that shows each exact command and its official source. Execution is limited to fixed catalog entries run through verified package managers (winget, brew, npm, flatpak) without a shell. Official install scripts (curl|sh, irm|iex) are shown as manual instructions and are never executed by GasaCLI. Nothing is installed silently, no license is accepted on the user's behalf, and sudo is never invoked automatically. Defaults pre-select Claude Code and Codex when missing; every selection is reversible before confirmation.

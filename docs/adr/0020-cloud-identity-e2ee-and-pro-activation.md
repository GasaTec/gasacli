# ADR-0020 — Cloud identity, E2EE sync and Pro activation

Status: Accepted design direction, 2026-10-28. Protocol, service and commercial-provider implementation remain pending.

## Decision

Keep local orchestration usable without an account or network. Cloud is optional hosting for opt-in synchronization and device administration; billing grants service entitlement but does not unlock local Community features. Use a self-hostable protocol and keep account/billing metadata separate from encrypted sync payloads. The detailed product design, unresolved choices and verification steps are in [`docs/08-cloud-sync-e-monetizacao.md`](../08-cloud-sync-e-monetizacao.md).

## Identity and device enrollment

Account authentication identifies the service user and is not an encryption key. Each installation has a revocable device identity and locally generated key material protected by the OS credential store where available. Enroll a new device only after an authenticated user action and an explicit device-to-device or recovery-key confirmation; do not use hardware fingerprints as the sole identity. Device removal blocks future service access but cannot erase ciphertext or plaintext already copied to a device.

## Sync and key lifecycle

Sync only selected, supported data. Encrypt on the client before upload with user-controlled key material; transport security supplements, but does not replace, E2EE. Version encrypted objects and operations, make uploads idempotent, and preserve concurrent edits as explicit conflicts rather than silently overwriting them. Provider credentials, OS-vault entries and source repositories are excluded by default.

Key rotation, loss and recovery must be explicit. Without a separately retained recovery secret or an explicitly chosen escrow design, losing every authorized device/key makes the encrypted data unrecoverable. A server-side account reset or billing support request must not silently restore decryption capability.

## Billing and Pro entitlement

Payment evidence is validated server-side and processed idempotently before an entitlement is issued. A short-lived, single-use activation code is atomically redeemed, then bound to the account; the client stores refresh credentials in the OS vault when available. Entitlements have revocation and bounded offline grace. Expiry or loss of Pro stops managed-service access after grace, never local sessions, backups or orchestration. The Cloud design records the redemption state machine and provider capability checks; current payment-provider capabilities are unverified.

## Quotas, privacy and self-hosting

Managed service must publish quotas, retention, deletion and pricing before launch. No quota values or payment capabilities are committed by this ADR. A compatible self-host may set its own quotas and retention, and must not require Gasatec identity or billing for local operation. Minimize account metadata, avoid plaintext content access, provide account/device/payload deletion paths, and complete privacy and LGPD review before production.

## Unverified external claims

Any claim about payment-provider subscriptions, webhooks, refunds, recurring billing, API or compliance support is **UNVERIFIED** until checked against current primary provider documentation and a sandbox integration; record date, exact product/region and test evidence. Hosting, key-store and regulatory capabilities are also deployment-specific and require review before launch. Do not present these design assumptions as available service features.

## Consequences

Local Community behavior stays independent of Cloud. Hosted operation has service-side account and billing data but should not gain plaintext sync access. Recovery, device revocation, conflict resolution, quotas and deletion are acceptance-critical protocol behavior, not marketing promises. No Cloud or payment implementation is implied by this design.

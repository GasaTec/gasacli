# ADR-0019 — Canonical permission policy and approvals

Status: Accepted design, 2026-10-28. Implementation is pending.

## Context

ADR-0013 and `docs/spec/03-seguranca-e-permissoes.md` use overlapping permission names. This ADR establishes the canonical identifiers, profile defaults, scope precedence, approval record and user-facing view. The identifiers are stable policy keys; localized labels may change without changing stored policy.

## Canonical permission vocabulary

| Key | Covers |
|---|---|
| `read_project` | Read files within the trusted project boundary. |
| `write_project` | Create, edit or delete project files. |
| `run_tests` | Run configured test, lint, typecheck and build checks. |
| `execute_commands` | Run other local commands and child processes. |
| `install_dependencies` | Install or update project/runtime dependencies. |
| `network_access` | Make network requests or allow network access to child processes. |
| `git_mutations` | Change repository state, including checkout, commit, reset, push and branch operations. |
| `read_vault` | Read explicitly selected second-brain/vault content. |
| `write_vault` | Create, edit or delete explicitly selected vault content. |
| `issue_write` | Create, edit or close issues and other configured tracker records. |
| `plugin_install` | Install or update plugins, skills or executable extensions. |
| `secret_access` | Read or use credential material from an approved OS credential provider. |

`network`, `manage_issues`, `manage_plugins` and `access_secrets` in earlier text are aliases only: migrate them to `network_access`, `issue_write`, `plugin_install` and `secret_access`. Unknown permission keys fail closed. Read-only issue access is not a separate grant in this baseline; a future need must be added through an ADR.

## Profile defaults

`allow`, `ask` and `deny` are the only policy values. The following table sets each profile's built-in default. Hard security constraints and configured scope rules still apply.

| Permission | Restricted | Balanced | Autonomous |
|---|---:|---:|---:|
| `read_project` | allow | allow | allow |
| `write_project` | ask | allow | allow |
| `run_tests` | ask | allow | allow |
| `execute_commands` | deny | ask | allow |
| `install_dependencies` | deny | ask | ask |
| `network_access` | deny | ask | ask |
| `git_mutations` | deny | ask | ask |
| `read_vault` | deny | ask | ask |
| `write_vault` | deny | ask | ask |
| `issue_write` | deny | ask | ask |
| `plugin_install` | deny | ask | ask |
| `secret_access` | deny | deny | ask |

These defaults are a design baseline, not a promise that arbitrary commands can safely be classified as tests. Destructive operations, credential disclosure, policy changes and actions outside the trusted project boundary remain subject to hard constraints and explicit approval regardless of profile.

## Scope precedence and evaluation

1. Non-overridable hard security constraints apply first. They can deny an operation or require fresh, scoped approval.
2. An explicit user rule sets the user's maximum grant. A user `deny` is final; a user `ask` cannot be relaxed by a narrower scope.
3. Project rules may narrow the user grant. They cannot turn a user `ask` or `deny` into `allow`.
4. Agent rules may narrow the project-effective grant. An agent cannot grant itself permission or change another scope.
5. If no rule exists at a scope, inherit the broader effective rule; if no scope supplies a rule, use the active profile default.

When applicable rules disagree, `deny` wins over `ask`, and `ask` wins over `allow`. An approval authorizes only the recorded operation, target and lifetime; it does not mutate policy. A policy edit requires an explicit user action in the local client, is audited, and is reevaluated before the next operation. Trusting a project is not a blanket command grant.

## Approval event

Approval records are append-only local audit events. The event must contain:

```json
{
  "schema_version": 1,
  "event_id": "uuid",
  "occurred_at": "RFC3339 UTC timestamp",
  "decision": "allow | deny",
  "actor": {"kind": "user | policy", "id": "local stable identifier"},
  "requester": {"agent_id": "stable agent identifier", "run_id": "run identifier"},
  "permission": "canonical permission key",
  "scope": {"kind": "project | resource", "id": "stable scoped identifier"},
  "operation": {"summary": "redacted human-readable action", "digest": "optional content digest"},
  "expires_at": "RFC3339 UTC timestamp or null",
  "policy_revision": "effective policy revision",
  "reason": "optional user-provided reason"
}
```

Do not store secrets, credential values, full environment variables or private command payloads in the event. The UI must show the operation and target before approval; omission of a digest is acceptable where a stable digest cannot be safely produced. An approval is never attributed to an agent/model.

## Effective-rules view and commands

`/permissions` and its Portuguese alias `/permissoes` open the same view. Render a permission table with canonical key, localized description, effective value, source scope (`hard policy`, `user`, `project`, `agent` or `profile default`), and any narrower override. Show the active profile and project identity at the top. Mark unresolved `ask` rules as approval-required; do not imply they are already approved.

The view supports inspecting a permission, changing the profile, setting/removing an explicit user/project/agent rule, and reviewing recent approval events. Mutations require user confirmation, state the scope and resulting effective value, and write an audit event. A project or agent view cannot edit a broader scope. The alias `/permissoes` uses Portuguese labels while preserving canonical keys in exports and diagnostics. If the UI cannot safely identify scope, it must show the effective value read-only.

## Consequences

The policy is deterministic and agent grants cannot exceed user intent. Implementations must migrate legacy aliases, fail closed on unknown keys, and test the evaluator separately from command classification and UI rendering. This ADR defines design only; it does not claim enforcement exists today.

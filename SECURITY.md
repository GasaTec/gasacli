# Security Policy

## Supported versions
GasaCLI is pre-release and has no supported production versions yet. Security fixes will be prioritized for the current development branch until versioned releases begin.

## Reporting vulnerabilities
**Do not open a public issue with exploit details, secrets or private user data.** Use GitHub's **Report a vulnerability** / private vulnerability reporting feature when enabled. If that feature is unavailable, contact the repository maintainers privately and request a secure reporting channel; do not publish exploit details.

We will acknowledge reports when maintainers are available, investigate impact, coordinate remediation and credit reporters with their consent. No fixed response-time SLA is promised at this stage.

## Security design commitments
- Local-first storage; cloud sync is opt-in.
- No plaintext secrets in repository files, logs or issue attachments.
- Store tokens using the OS credential store when supported.
- Require trust approval before accessing a project and explicit consent for sensitive actions.
- Treat model responses, source files, Markdown and third-party skills as untrusted.
- Isolate child agents and restrict permissions according to user policy.
- Avoid uploading code, conversations or vault contents by default.
- Avoid silently installing plugins or executing fetched scripts.
- Cloud sync must be designed for end-to-end encryption and transparent recovery limitations.

## Disclosure
Please allow maintainers a reasonable opportunity to investigate and release a fix before public disclosure.

# Security policy

Security fixes target the latest release. Older releases may require upgrading;
this early-stage project does not maintain separate long-term support branches.

## Reporting a vulnerability

Use GitHub's [private vulnerability report form](https://github.com/lfreixial/plane-cli/security/advisories/new).
Include the affected version, reproduction steps, impact, and a sanitized proof
of concept. Do not open a public issue containing exploit details or credentials.
If private reporting is unavailable, open an issue requesting a private contact
channel without including vulnerability details.

## Credential handling

Plane personal access tokens authorize API operations as their owner. Use a
dedicated account with only the workspace/project access needed for your task.
Use HTTPS for remote instances.

Interactive setup stores the entered token in a local JSON config file. The
token is not encrypted; file permissions are restricted to `0600` on Unix.
On Windows, protect the config with your account's filesystem access controls.
Environment tokens are not saved unless `--save-token` is explicitly requested.
`plane config show` redacts the token, but review all output before sharing it.

If a token is exposed, revoke it in Plane and create a replacement. Removing
the token from a file or Git history does not invalidate it.

CI and weekly scans use `govulncheck` on supported OS/architecture targets.
A passing scan only covers known advisories and is not a security guarantee.

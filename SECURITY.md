# Security Policy

## Supported versions

Security fixes are made for the latest released minor version.

| Version | Supported |
| ------- | --------- |
| 0.1.x   | Yes       |

## Reporting a vulnerability

Please **do not** open a public issue for security problems.

Report privately using GitHub's "Report a vulnerability" button on the repository's Security tab, or email **jahidhasann67@gmail.com**. Include:

- A description of the issue and its impact
- Steps to reproduce, or a proof of concept
- The Portpeek version (`portpeek version`) and your operating system

You can expect an acknowledgement within 5 business days. We will keep you informed of progress and credit you in the release notes if you wish.

## Scope notes

Portpeek reads local process and socket information and can send signals to processes. Reports about the following are especially welcome:

- Signalling a process that the safety rules should have refused (PID 0/1, portpeek itself, container-owned ports)
- Privilege escalation through the `kill` command or the Docker endpoint handling
- Unexpected network connections (the only intended connection is to the configured Docker endpoint)

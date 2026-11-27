# Security policy

Tessaly runs shells and coding agents with your user's rights. We treat any bug that lets someone else reach the Tessaly API or your files as critical.

## Report a vulnerability

Do not open a public issue for security problems.

- Email security@tessaly.dev, or
- Use GitHub's private vulnerability reporting ("Report a vulnerability" on the repository's Security tab).

Please include:

- The version or commit you tested
- Your OS and architecture
- Steps to reproduce, and what an attacker gains
- Any proof of concept code

## What happens next

- We confirm receipt within 3 working days.
- We send an assessment and a planned fix date within 10 working days.
- We fix the issue in a private branch, publish a GitHub security advisory, and credit you unless you ask us not to.
- We ask you to keep the details private until the fix ships, and for no longer than 90 days from your report.

## Supported versions

Only the latest release gets security fixes.

## Scope

In scope: the Tessaly daemon, the `tess` CLI, the web UI, and official release binaries.

Out of scope: bugs in the agent CLIs Tessaly launches (Claude Code, Codex and others), and setups that expose Tessaly to a network against the guidance in the README. Report agent bugs to their own maintainers.

## Safe harbour

We will not take legal action against research done in good faith under this policy. Test only against your own installs, and do not access or change other people's data.

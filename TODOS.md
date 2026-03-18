# TODOs

## P2: OpenClaw skill — unify with hosted API

**What:** Update openclaw-skill/ to support both self-hosted and hosted API (same pattern as anakin-cli unified CLI).

**Why:** OpenClaw is a distribution channel for AI agents. If the skill works against hosted out of the box with an API key, every OpenClaw user who installs it is one API key away from being a customer.

**How to apply:** Add ANAKIN_API_URL and ANAKIN_API_KEY env var support. When API key is present, route to hosted. When absent, route to localhost:8080.

**Effort:** S
**Depends on:** Unified anakin-cli changes (completed)

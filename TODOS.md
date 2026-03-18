# TODOs

## P2: OpenClaw skill — unify with hosted API

**What:** Update openclaw-skill/ to support both self-hosted and hosted API (same pattern as anakin-cli unified CLI).

**Why:** OpenClaw is a distribution channel for AI agents. If the skill works against hosted out of the box with an API key, every OpenClaw user who installs it is one API key away from being a customer.

**How to apply:** Add ANAKIN_API_URL and ANAKIN_API_KEY env var support. When API key is present, route to hosted. When absent, route to localhost:8080.

**Effort:** S
**Depends on:** Unified anakin-cli changes (completed)

## P3: Cache compiled regexes in domain config

**What:** Compile regex patterns (failure_patterns, required_patterns) once at DomainConfig load time instead of on every job check.

**Why:** Currently `regexp.Compile()` is called on every job for every pattern in domain/detector.go. For 100 jobs with 5 patterns, that's 500 compilations. Caching at load time gives 10-50x throughput improvement for regex-heavy configs.

**How to apply:** Add `[]*regexp.Regexp` fields to DomainConfig struct. Compile patterns in repository.go when loading from DB. Use pre-compiled regexes in detector.go.

**Effort:** S (human: ~2 hours / CC: ~10 min)
**Depends on:** Nothing

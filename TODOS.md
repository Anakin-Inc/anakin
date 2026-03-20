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

## P2: Telemetry — version check via response

**What:** When `telemetry.anakin.io` responds to the hourly POST, include `{"latest_version": "X.Y.Z"}` in the response body. Server compares to its own version and logs "New version available" if behind.

**Why:** Free upgrade signal for self-hosted users + version distribution data for the team. Mutual value exchange that makes telemetry feel less one-sided.

**How to apply:** Parse the response body in `trySend()`. Compare `latest_version` to `serverVersion`. Log at INFO level if a newer version is available.

**Effort:** S (human: ~2 hours / CC: ~10 min)
**Depends on:** Telemetry receiver being built at telemetry.anakin.io

## P2: Telemetry — error category taxonomy

**What:** Classify telemetry error counts into fixed categories (`timeout`, `blocked`, `dns_failure`, `parse_error`, `browser_crash`, `connection_refused`, `rate_limited`, `unknown`) instead of just total error count.

**Why:** Actionable error analytics — know which failure modes are most common. Also a privacy improvement: no risk of raw error strings leaking URLs or internal details.

**How to apply:** Add an `ErrorCategory` field to `telemetry.Event`. Map error strings to categories in `processor.go` before emitting. Add per-category atomic counters in the collector.

**Effort:** S (human: ~3 hours / CC: ~15 min)
**Depends on:** Nothing

## P3: Telemetry — verbose audit mode

**What:** Add `TELEMETRY=verbose` that logs the full JSON payload to stdout (via `slog.Debug`) before each HTTP send. Power users can audit exactly what leaves their machine.

**Why:** Maximum transparency for security-conscious admins who want to verify telemetry claims without reading source code.

**How to apply:** In `trySend()`, check if verbose mode is enabled. If so, log the marshaled payload at DEBUG level before sending.

**Effort:** S (human: ~2 hours / CC: ~5 min)
**Depends on:** Nothing

## P3: Telemetry — first-boot dry-run

**What:** On the first telemetry cycle after installation, log the payload but don't send it. Print: "Telemetry dry-run complete — see /v1/telemetry/status. Next cycle sends for real. Disable: TELEMETRY=off".

**Why:** Gives privacy-conscious users a grace period to review what will be sent before any data leaves their machine.

**How to apply:** Track a `firstRun` boolean in the collector. On first `trySend()`, log the payload and return without POSTing.

**Effort:** S (human: ~2 hours / CC: ~10 min)
**Depends on:** Nothing

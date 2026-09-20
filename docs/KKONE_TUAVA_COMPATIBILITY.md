# KKONE / Tuava compatibility merge

## Scope and provenance

This is a local compatibility merge, not a production deployment. No production
database migration or live model request was performed during this merge.

- Custom baseline: `77589ea62dd29b08d6332804a188c4c2b89c2fcc`, from
  `1226287131-ui/sub2api`, branch `codex/sub2-retry-quality-audit-20260912`.
- Imported tree: `1607ca38d6fdb92abcaae108d1ac6c1530be0687`, from
  `Tuava/sub2api-codex-ticket`, version `0.2.7-tuava.2`.
- Common ancestor: `270eac6973049fe1b50eb75560a74a029e82884c`.
- Compatibility version: `0.2.7-tuava.2-kkone.1`.
- Working branch: `codex/sub2-tuava-compat-20260920`.

Use the custom baseline above, not the older custom repository `main` at
`0be0abe7c`. The latter lacks the later cache-creation billing and WebSocket
sanitization fixes. The merge uses real Git parent history, not a source-tree
replacement. Original project licenses and Tuava's `NOTICE.md` are retained.

## Retained custom behavior

| Feature | Compatibility contract |
| --- | --- |
| Early OpenAI stream priming | A non-semantic SSE frame can precede account-slot waiting. It does not issue an extra model request or add tokens. |
| Failover after priming | Priming/keepalive bytes alone do not count as model output and must not disable existing failover. |
| Dynamic account waiting | Candidate competition acquires concurrency slots only; it does not race duplicate model generations. The winning account is used for forwarding. |
| Scheduling | Existing scoring, sticky escape, shared account concurrency controls and bounded retry/backoff remain. |
| Cache creation as input | The per-channel/platform switch remains in the UI, client response rewriting and local usage billing. |
| OpenAI token normalization | Total input already includes cache creation, so clear the creation buckets without adding their tokens again. |
| Anthropic token normalization | Add `max(aggregate_creation, creation_5m + creation_1h)` once to ordinary input and clear creation buckets. Read-cache and output categories remain unchanged. |
| Billing policy snapshot | Capture the policy during forwarding and normalize a result copy during settlement, preserving existing request-ID idempotency. |
| Protocol coverage | Keep HTTP, SSE, converted protocols and WebSocket client-frame usage sanitization. |
| Mapped Astra requests | Preserve `reasoning.mode` according to the mapped upstream model. |

Primed first-byte time is not the real time to the first model-generated token.
These must not be treated as interchangeable latency measurements.
The existing cache-creation policy covers the text gateway paths listed above;
the direct Images forwarding path did not use that policy before this merge and
has not been newly brought into its scope here.

## Imported behavior

- Tuava Codex-ticket configuration, capture, persistence, status display, manual
  probes, background refresh and per-account/model missing-ticket policies.
- Account proxy pools and independent proxy lanes with concurrency, weight,
  timeout and circuit settings; round-robin, least-connection and weighted modes.
- Smart proxy assignment, bulk settings and managed import profiles.
- The upstream changes included in Tuava's pinned tree, including the newer
  frontend/API contracts and OpenCode provider support.

## Compatibility adjustments

- Keep both custom stream-priming settings and Tuava ticket settings.
- Keep multi-candidate waiting and use effective enabled proxy-lane capacity
  consistently when constructing waiting plans.
- Preserve the original concurrency behavior for legacy accounts without an
  explicitly configured proxy pool/lane list. Loading or editing an old account
  alone must not silently turn its concurrency setting into a one-slot lane.
- Preserve dynamic selected-account revalidation in Grok media handling while
  retaining the new Seedance and bound-video ownership behavior.
- Preserve the cache-creation wrapper and the newer OpenCode inbound-body capture
  in the Anthropic conversion path.
- Evaluate ticket eligibility using the real compact outbound model throughout
  selection, snapshot refresh, database rechecks and waiting fallback. Compact
  capability checks remain separate, preserving the specific unsupported-compact
  error and releasing any acquired slot when a candidate is rejected.
- Retain validated waiting candidates when the database-recheck budget is
  exhausted instead of incorrectly reporting that no account is available.
- Keep a plugin lane's timeout context separate from the fallback HTTP request.
  If the plugin binding is disabled between route checks, releasing that lane
  must not cancel the ordinary upstream fallback. The regression checks a single
  fallback call, unchanged body, lane release and caller cancellation propagation.
- Align older retry tests with the existing shared `Retry-After` cooldown policy:
  an explicit cooldown must not become a private immediate same-account retry.
  Protocol tests cover both the transient and explicit-cooldown branches.
- Make SSE keepalive tests use a virtual clock and give the stale Ollama callback
  fixture a distinct generation timestamp, removing wall-clock precision races
  without changing the production behavior.

The default compact model follows the imported version (`gpt-5.5`); explicitly
configured compact-model values are not overwritten.

## Defaults and limitations

- Stream priming remains enabled by default.
- The imported WebSocket connection-pool factors default to 5.0 instead of 1.0,
  still bounded by `max_conns_per_account`. These size live session connections,
  not the number of simultaneously executing account requests. Existing explicit
  configuration overrides the defaults; assess socket and memory use at rollout.
- Ticket harvesting remains disabled by default; fail-closed is also disabled
  by default. Existing database settings can override defaults on deployment.
- Ticket probes are real upstream requests and can consume quota. They are not
  ordinary customer usage entries. Do not describe harvesting as zero-cost or
  enable it silently during deployment.
- Ticket length/prefix/TTL checks are local heuristics, not proof of model quality
  or an exemption from upstream rate limits. No quality or capacity claim has
  been validated against a real account here.
- This merge does not supply European IPs, automatically change egress regions,
  or guarantee that a geographic route changes model quality.
- Proxy-lane counters and ticket singleflight are process-local. Existing Redis
  account-level limits remain separate; lane limits are not distributed locks.
- The imported background harvester can start one probe per eligible
  account/model without a global probe concurrency cap. Keep it disabled until
  its traffic and capacity are assessed for the intended account pool.
- Tuava's automatic upstream-sync workflow remains repository-gated to Tuava's
  own repository; it is not silently enabled for this fork.

## Database and rollout boundary

Two upstream SQL migrations are included:

1. `238_opencode_go_platform.sql` extends platform/provider constraints.
2. `238_purge_unlimited_user_platform_quotas.sql` deletes platform-quota rows whose
   daily, weekly and monthly limits are all NULL. It does not delete usage logs,
   balances or customer records, but it is still a database mutation.

The migration runner keys migrations by full filename and checksum, not just the
numeric prefix, so the shared `238` prefix is not a duplicate migration key.
Existing migration files were not changed by this merge.

Before production rollout: back up the current database, restore it into an
isolated test environment, rehearse migrations and representative billing paths,
then use a controlled rollout with a defined rollback boundary. A code build
does not prove that production database migrations or live upstream behavior are
safe. No migration has been applied to a production database in this task.

## Local verification

Completed on 2026-09-20:

- Full backend unit suite: `go test -tags=unit -p 2 ./...` passed. Git's `sh`
  was available on PATH for repository shell-script tests.
- Full frontend unit suite: 299 test files and 2,247 tests passed.
- Frontend production build, including i18n checks and Vue type checking,
  passed. Vite reported its non-fatal large-chunk warning.
- Native backend build: `go build -p 2 ./...` passed.
- Linux amd64 release build with embedded frontend and `CGO_ENABLED=0` passed.
- Focused regressions cover compact ticket selection, old-account lane opt-in,
  multi-candidate waiting, shared cooldown handling and deterministic SSE timing.
- No unresolved merge conflicts or staged whitespace errors remained at review.

Test traffic uses fixtures/stubs rather than customer API keys or live model
calls. These checks do not include a production-database restore/migration
rehearsal, a live upstream quality evaluation or a real concurrency load test.
The merge is local only: no GitHub push or production rollout was performed.

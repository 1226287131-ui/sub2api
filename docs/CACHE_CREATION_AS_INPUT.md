# Cache creation billed as regular input

The existing channel feature `features_config.hide_cache_creation[platform]`
now controls both client-visible usage and local billing for **new requests**.
The UI labels it “缓存创建并入普通输入 / Bill cache creation as regular input”.
It applies only to groups associated with the active pricing channel, using the
selected account's platform. False/unset preserves existing behavior.

## Accounting contract

- OpenAI `prompt_tokens` / `input_tokens` are inclusive totals. Do not add cache
  writes again. Remove creation breakdown fields; local regular input is total
  input minus cache reads. Output and cache-read usage/prices are unchanged.
- Anthropic `input_tokens` excludes cache writes. Add the larger of the aggregate
  creation count and the 5-minute + 1-hour breakdown once, then remove/zero every
  creation bucket. The two TTL buckets are components, not extra token charges.
- Local usage logs, cost calculation, quota/balance settlement and downstream
  usage use this classification. Each site still uses its own configured input
  prices and multipliers; this option does not change a downstream price list.
- The forwarding-time true/false policy is carried into asynchronous settlement.
  Raw upstream usage is retained in the result; normalization operates on copies,
  preserving existing billing request IDs and deduplication. Historical invoices
  and database rows are not rewritten.

Example: OpenAI total input 5,447, cache read 5,185, creation 259 becomes local
regular input **262**, read **5,185**, creation **0**. Client total input stays
**5,447** (not 5,706).

## Covered response paths

HTTP Chat Completions, Responses, Messages, raw passthrough, protocol conversion,
buffered SSE-to-JSON, terminal usage chunks, and OpenAI WebSocket forwarding.
Aliases include `cached_creation_tokens`, `cached_creation_input_tokens`,
`cache_creation_tokens`, `cache_creation_input_tokens`, `cache_write_tokens`,
`cache_write_input_tokens` and Anthropic TTL breakdowns. Only protocol usage
envelopes are transformed; model content and tool payloads are not traversed.

## Verification

`go test -tags unit ./internal/service ./internal/pkg/apicompat ./internal/service/openai_ws_v2 -run 'Test.*(CacheCreation|CacheVisibility|SanitizeCache|RawChat|ChatCompletions|HandleSSEToJSON|PassthroughSSEToJSON|ExtractOpenAIUsage|ResponsesUsage|RecordUsage)' -count=1`

Regression cases cover real forwarding entry points against mock upstreams,
enabled/disabled channel configuration, streaming and buffered responses,
immutable raw usage, input/read/output totals, local cost/settlement commands,
retry deduplication, platform switches, integer bounds, and output preservation.
Frontend locale completeness, Vue typecheck and production build are verified.

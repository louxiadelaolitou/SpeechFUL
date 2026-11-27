# Recommendation core

`transport/` defines bounded AES-GCM references. This branch ships the pure
ranking, round model, transport codec and offline experiment. Backend wiring
and acceptance status are tracked in the
[recommendation handoff](../../../../docs/tasks/AB-final-acceptance.md).
The selector remains pure; the following B1 description records its scope.

## B1 experimental recommendation rules

This package is a pure, internal experiment. It has no database, HTTP, mobile UI,
real-account collection, round persistence, cursor codec, or authorization hook.
It is not enabled by any existing product endpoint. Its numeric defaults are
experiment inputs, not platform parameters, approved retention policy, or optima.

`Select(Config, Input)` accepts a fixed `Now`, seed, channel scope, bounded
candidate catalog, four independent source memberships, optional bounded positive
interest signals, and finite round/delivery/exposure summaries. It returns chosen
IDs and INTERNAL feature/selection explanations. Its output is not a public DTO.
It neither mutates input nor registers delivery, media prefetch, exposure, or a
body view. A caller must update summaries explicitly; identical input repeats.

The catalog and raw source lists must fit configured caps; oversize input is
rejected, never silently truncated. Conflicting duplicate catalog IDs, invalid
timestamps, unknown channels, malformed grouping keys, NaN, and unbounded state
fail closed. Repeated membership IDs are deduplicated, including overlaps across
all sources. Real recall eligibility, per-source time windows, query deadlines,
bounded SQL scans, and final access checks belong to later adapters, not this
package. Candidate `Eligible` is not proof that an actual read is authorized.

## Rules

- Freshness is a fixed-scale exponential half-life. Interest is decayed positive
  mass with a 0.5 prior, compared with average mass on the other allowed channels;
  absent/expired history returns to neutral. It never renormalizes on batch
  extrema. The caller must deduplicate real positive facts and apply withdrawals;
  B1 synthetic signals are not an ingestion or anti-abuse protocol.
- Heat uses eligible events in `[now-window, now]`, with the lower boundary
  excluded. It groups private actor keys across interaction kinds, excludes the
  supplied owner, counts a like/favorite once per actor, caps each actor's total,
  then caps and log-compresses the sum. `Participants` counts distinct eligible
  actors, not comments or public masks. Correct account-to-key mapping and current
  moderation/root visibility are adapter responsibilities. No historical like
  timestamp is invented. Heat has no extra post-age multiplier.
- Quotas schedule sources in interleaved order (initially R/I/D/E/R/I/D/R/I/R).
  Try the preferred source then cycle through the other sources. Source is the
  actual selected route, not a mutually exclusive post label. Quotas are soft;
  a zero quota does not disable a source as a shortage fallback.
- Each source sorts by score/ordinal/ID, except independent exploration, which
  sorts by a fixed seed/ID digest, then score/ordinal/ID. Exploration is never
  truncated to a high-score union. This is a deterministic experimental rotation,
  not a fairness or eventual-exposure guarantee.
- Exact current-round IDs, ineligible candidates, and active exposure cooldown
  are hard exclusions. First select unseen IDs outside delivery cooldown under
  soft channel/author caps; when impossible relax diversity, then recycle only
  delivered-but-unexposed IDs. Return fewer than a page when necessary. Previously
  exposed IDs may return after the configured cooldown in an explicit new round;
  exact current-round IDs remain excluded even when their cooldowns expire.
- After membership selection, spread adjacent channels/public author groups
  within each relaxation phase. Prefer differing author and channel, then author,
  then channel; preserve prior order for ties. This does not promote recycled
  delivery before available new IDs. Diversity is a preference, not a promise for
  single-channel/author pools or a proof that greedy spreading is globally optimal.

No new history state is returned or silently evicted. Round exhaustion and
`HasMoreWithinInput` describe these finite inputs only, never global EOF or
durable pagination. Deterministic selection with fixed state does not guarantee
stable retries when candidate eligibility, time, features, or summaries change.
B2 must design frozen page membership/boundaries, intent replay, current
authorization, capacity, retention, cleanup, and rollback before product wiring.
Never regenerate a page from changed ranking inputs and call it a stable retry.

## Reproduce without services

From `services/api` with Go 1.22.2 or a compatible toolchain:

Prepare the locked modules before offline checks, including after a cache miss.
The recursive target includes transport's `posts` dependencies; a restored cache
is only an optimization. Preparation retains go.sum and checksum-database
verification and does not upgrade dependencies or the toolchain.

```sh
export GOMAXPROCS=2 GOTOOLCHAIN=local GOSUMDB=sum.golang.org
GOPROXY=https://proxy.golang.org go mod download
go mod verify
git diff --exit-code -- go.mod go.sum
export GOPROXY=off
go test -mod=readonly -race -count=1 ./internal/recommendation/... ./cmd/recommendationlab
go build -mod=readonly ./internal/recommendation/... ./cmd/recommendationlab
go vet -mod=readonly ./internal/recommendation/... ./cmd/recommendationlab
go run -mod=readonly ./cmd/recommendationlab > /your/task-output/synthetic-results.json
```

`recommendationlab` accepts no arguments or user input. Its 30 deterministic cases
use five fictional datasets and six configuration variants. Config durations in
its JSON are nanoseconds (`time.Duration`); full config/input hashes accompany
each case. The chronological comparator uses the same fictional candidate
catalog, not an unbounded production timeline. Adjacent overlap divides by the
smaller effective batch; an empty denominator must not become a fabricated zero
rate. Only the first two IDs receive simulated exposure; other delivered IDs can
recycle when the pool is small. This measures synthetic rule behavior, not user
benefit, real exposure coverage, production SQL/latency, or capacity.

The golden JSON is an algorithm fixture. The Feed backend workflow runs this
package through its all-package Go test, build and vet steps. Use the commands
above for the focused race and offline experiment checks.

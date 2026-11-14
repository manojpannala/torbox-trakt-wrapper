# ADR-0001: The metadata cache is a concrete generic store behind a closed key set

- **Status:** Accepted, **amended by `0003-cache-orchestration.md`** — read that
  first. It renames `TraktWatched` to `TraktCatalog`, adds `Clear()`, adds a
  version to the envelope, and replaces the fixed-`.tmp` write with
  `os.CreateTemp`. The `Store` contract below otherwise stands.
- **Date:** 2026-09-12
- **Affects:** `pkg/cache` (new), `internal/tui/app.go`, `internal/cli/list.go`
- **Implements:** FR-3.1.1 – FR-3.1.5
- **Supersedes:** the `Store` contract in DESIGN §2.2 as
  originally written (that section has been amended to match this ADR)

## Context

Phase 3 introduces `pkg/cache`, the keystone of v0.3.0: Phases 4 and 5 and
several deferred features all need somewhere to persist data, and
`torbox.cache_ttl_minutes` has been a config field nothing reads since v0.1.0.

DESIGN §2.2 originally specified:

```go
type Store interface {
    Get(key string, out any) (fresh bool, err error)
    Put(key string, v any) error
    Invalidate(key string) error
}
```

Designing against the real call sites turned up four problems.

**The result triple cannot express the outcome.** `Get` returning
`(false, nil)` means either "stale hit, `out` is populated" or "miss, `out` is
untouched". Those demand opposite behaviour from the caller, so the rule that
distinguishes them is part of the interface but lives outside the signature.

**One adapter.** The only implementation is on disk, and `TASKS.md` commits to
testing through `XDG_CACHE_HOME` — a real store in a temp directory, not a
fake. An interface with one adapter is a hypothetical seam.

**`out any` is a workaround for a Go rule.** Methods cannot take type
parameters, so a generic `Store.Get[T]` is impossible. Package-level generic
functions over a concrete struct restore type safety.

**`bypassCache` is already taken.** All six `pkg/torbox` getters take a
`bypassCache bool` for TorBox's *server-side* cache, and all seven call sites
hardcode `true`. FR-3.1.5's "explicit refresh bypasses the cache" is a
different cache, and reusing the word would guarantee confusion.

Separately, an audit of the four payloads confirmed that `Torrent`,
`TorrentFile`, `UsenetItem`, `WebDLItem` and the Trakt watched models carry no
signed URL and no credential. `Link`, `AuthID` and `URL` appear only on the
create/control request and response types and on `DownloadLink`, none of which
is a listing. So the payloads are safe to cache — but nothing in the design
*kept* them safe as the code grew.

## Decision

```go
package cache

// Key names a cacheable payload. The set is closed: adding a key is a
// deliberate assertion that the payload contains nothing signed or secret.
type Key string

const (
    TorBoxTorrents Key = "torbox-torrents"
    TorBoxUsenet   Key = "torbox-usenet"
    TorBoxWebDL    Key = "torbox-webdl"
    TraktWatched   Key = "trakt-watched"
)

// Entry is a decoded cache hit.
type Entry[T any] struct {
    Value    T
    StoredAt time.Time // drives the UI's "updated 3m ago" stale marker
    Fresh    bool      // within TTL
}

// Store is a TTL'd JSON cache under a directory. Never fatal: any I/O or
// decode failure is logged at debug and reported as a miss.
type Store struct{ /* dir, ttl, log */ }

func New(dir string, ttl time.Duration, log *slog.Logger) *Store

// Read reports ok=false when there is nothing usable to show.
// A stale hit returns ok=true with Entry.Fresh=false.
func Read[T any](s *Store, key Key) (Entry[T], bool)

// Write and Invalidate are best-effort; failures are logged, not returned.
func Write[T any](s *Store, key Key, v T)
func (s *Store) Invalidate(key Key)
```

Five changes against the original contract:

1. `Store` is a concrete struct, not an interface. One adapter, no seam.
2. `Read[T]` / `Write[T]` are package-level generic functions, not methods
   taking `out any`. Go forbids type parameters on methods.
3. `Write` and `Invalidate` return nothing. Under §2.4 every cache error is
   non-fatal, so a returned error has exactly one possible caller response,
   `_ =`. The `*slog.Logger` is where the information goes instead.
4. Keys are a closed `Key` enum, not free-form strings.
5. `Entry.StoredAt` is exposed, so the UI can say how stale a stale render is.

**No read-through helper.** A `Through(ctx, key, fetch)` looks deeper and would
break FR-3.1.2: a warm start needs the stale value now and the fresh one later,
which is two emissions, and a read-through blocks on the network for a cold or
stale entry. The TUI already models two-phase loading as two Bubble Tea
messages, so `Read` stays synchronous and network-free — which is exactly what
makes the warm start possible.

**Seam placement.** Not inside `pkg/torbox`, as DESIGN §2.1 already said, and
for a reason worth stating: performance characteristics are part of an
interface. Caching inside `GetTorrents` would leave the signature identical
while making its latency unpredictable — the interface gets worse while
appearing unchanged. `pkg/torbox` and `pkg/trakt` stay pure API clients; the
cache sits at the call sites in `internal/tui` and `internal/cli`.

## Consequences

**FR-3.1.5 costs no interface surface.** Explicit refresh simply does not call
`Read`; it fetches and `Write`s. There is no bypass flag, so no collision with
TorBox's `bypassCache`.

**§2.5 becomes structural rather than remembered.** A stream URL cannot be
cached without adding a `Key` constant, and that is a visible diff which
invites the question. The rule lives in one place instead of in every future
caller's head.

**Tests need no clock.** Because §2.3 evaluates TTL from `stored_at` in the
file content rather than from mtime, a fresh/stale test just writes an envelope
with a chosen timestamp. No injected clock, no internal seam, no sleeping. The
four `TASKS.md` cases (fresh / stale / missing / corrupt) plus a `0600`
assertion and a marshal check that no `url` or `token` key appears.

**Ordering dependency.** `New` takes a `*slog.Logger`, so structured logging
must land first. `ROADMAP.md` §5 already sequences it that way (step 2, before
Phase 3 at step 3).

**Cost: `T` cannot be inferred.** Call sites read
`cache.Read[[]torbox.Torrent](store, cache.TorBoxTorrents)`. Accepted as the
price of type safety over `out any`.

**If a second adapter ever appears** — an in-memory store for a `--no-cache`
mode, say — extract the interface then. `cache_ttl_minutes = 0` does not need
one; the TTL arithmetic makes every entry stale, which is the correct
behaviour already.

## Alternatives considered

- **Keep the specified `Get(key, out any)`.** Rejected: the `(false, nil)`
  ambiguity, and no type safety.
- **Read-through `Through(ctx, key, fetch)`.** Rejected: breaks FR-3.1.2, as
  above.
- **Cache inside `pkg/torbox`.** Rejected: makes client latency
  unpredictable, and the Trakt catalog needs the same treatment anyway.
- **Interface with a disk adapter and a memory fake.** Rejected: one real
  adapter, and `TASKS.md` chose real-disk-in-tempdir testing deliberately.

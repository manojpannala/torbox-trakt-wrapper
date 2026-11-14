# ADR-0003: How the metadata cache is orchestrated, kept current and cleared

- **Status:** Accepted and shipped in **v0.3.0** on 2026-09-26 (PR #1), with
  follow-ups in v0.3.1 (PR #8).
- **Date:** 2026-09-26
- **Amends:** `0001-metadata-cache-interface.md` — its `Store` contract stands;
  this ADR changes the key set, the envelope and the write mechanics, and
  decides everything ADR-0001 left to the caller
- **Affects:** `pkg/cache` (new), `internal/tui/library.go` (new),
  `internal/tui/app.go`, `internal/cli/auth.go`, a new `internal/cli/cache.go`
- **Implements:** FR-3.1.1 – FR-3.1.9 (SPEC §3.1)

## Context

ADR-0001 designed the `pkg/cache` interface in isolation. Returning to build it
two weeks and three releases later, five things had changed or come to light.

**The resume prompt now reads playback positions.** ADR-0001 predates it. A
warm start renders from disk before the network answers, so a cached position
could offer "Resume at about 40%" when the item has since been watched to 70%
on another device — and a plausible wrong number gives the user no reason to
doubt it.

**The Trakt catalog is three calls, not one.** `fetchTraktCatalogCmd` gets
watched movies, watched shows and playback. ADR-0001 defined a single
`TraktWatched` key and never considered playback.

**TorBox's slowness is mostly self-inflicted.** Measured 2026-09-26 against a
462-torrent library, `GET /torrents/mylist`:

| | run 1 | run 2 | run 3 |
| --- | --- | --- | --- |
| `bypass_cache=true` (every call today) | 6278ms | 3623ms | 8530ms |
| `bypass_cache=false` | 1552ms | 1653ms | 1034ms |

TorBox's server-side cache is 4–6× faster and far more stable, and the app has
been bypassing it on every call including startup.

**Switching tabs never fetches.** `Init` loads only the active tab, and the
tab keys only change `activeTab`. Usenet and Web-DL stay empty until visited
*and* refreshed with `r`. With a cache this would get worse: those tabs would
render old data that nothing ever refreshes.

**`app.go` is 1315 lines**, 20% past the 1096 at which Phase 6.1 scheduled
splitting it.

## Decision

### Key set

Four keys, still closed:

```
TorBoxTorrents   TorBoxUsenet   TorBoxWebDL   TraktCatalog
```

`TraktWatched` becomes **`TraktCatalog`**: one envelope holding watched movies,
watched shows and playback, because they are always fetched and consumed
together. `PlaybackItem` was audited for this — id, progress, paused_at, type,
and title/id objects; no URL, no credential.

### Envelope and writes

```json
{"version": "<app version>", "stored_at": <unix>, "payload": ...}
```

- **A version mismatch is a miss.** JSON decoding is lenient, so a renamed or
  added field on `torbox.Torrent` would otherwise decode an old file without
  error into partially-zeroed structs — `0 B/s` speeds, nothing logged. The
  version is `config.Version`, **not** a hand-bumped schema number: nobody will
  remember to bump an integer when editing a model struct. Cost: one ~1.4s cold
  start per release.
- **Writes use `os.CreateTemp` in the target directory, then rename.** This
  supersedes ADR-0001's "the same dance `Config.SaveToFile` uses": that uses a
  fixed `.tmp` name, so two processes writing one key (the TUI and a CLI
  command, or two TUIs) could clobber each other's temp file and rename a torn
  one into place.
- **`Store` gains `Clear()`**, for `tt-wrapper cache clear`. Re-pairing uses the
  existing per-key `Invalidate`.
- **A successful empty payload is written like any other.** Treating "returned
  nothing" as "failed" would resurrect deleted torrents on every launch.

### Orchestration lives in `internal/tui/library.go`

Not in `app.go`. One file owns reading the cache, fetching, writing, and the
resume deferral; `app.go` gains about fifteen lines. This keeps ADR-0001's seam
— the TUI's call sites, not inside the API clients — while gathering the logic
in one testable place, and it works with Phase 6.1's split rather than against
it.

Cached data is **seeded synchronously in `NewAppModel`**, not delivered as
messages. The first frame therefore already shows the library, and there is
no race in which an asynchronously-read cached message lands after the
network one and overwrites fresh data with stale. The model tracks a per-tab
`cachedAt time.Time` — non-zero means the tab is showing cached data from that
time — which drives the staleness hint. (Planning 2026-09-26 replaced an
earlier idea of adding `StoredAt` to the four `…LoadedMsg` types.)

### When data is fetched

| Situation | TorBox | Trakt |
| --- | --- | --- |
| Launch | Every tab rendered from disk; **active tab always reconciles**, `bypass_cache=false` | Rendered from disk; **always reconciles** |
| First time a tab is shown this session | Reconciles **only if older than `torbox.cache_ttl_minutes`**, `bypass_cache=false` | — |
| `r` | Active tab, `bypass_cache=true` | Reconciles |
| `PlaybackFinishedMsg` | — | Reconciles (already did) and now writes the cache |

**The active tab ignores TTL at launch** because this is a download manager:
relaunching within 15 minutes and seeing a stale `12 MiB/s, ETA 5m` presented
as current would mislead. TTL governs tabs you switch to.

**Trakt ignores TTL entirely.** It is cheap (~0.9s for all three calls) and it
drives the resume prompt. The setting lives under `[torbox]` in the config, so
its name already says which data it governs.

### Resume deferral

Until the **first network** Trakt catalog of the session arrives, Enter on
**any** item is held — status "Checking your position…" — and replayed when it
lands, so the prompt uses the fresh position.

- It applies to every item, not only ◐ ones. An item cached as unwatched may
  have been started on another device since; deferring only ◐ items would play
  it from the beginning.
- **It always ends.** A failed Trakt fetch marks the session settled and
  replays the held Enter against the cached position. Offline never hangs.
- It only applies when Trakt is configured and authenticated.
- At most one launch is held. A second replaces it, and Esc cancels it.

### Retention

| Event | Effect |
| --- | --- |
| `tt-wrapper auth torbox <key>` | Invalidates the three TorBox keys |
| Trakt pairing, CLI or the TUI's `A` | Invalidates `TraktCatalog` |
| Token refresh | Nothing — same account |
| `tt-wrapper cache clear` | Removes everything |

No account's cached data is ever rendered to another, and there is a deletion
path, which the 2026-09-12 security sweep recorded as missing.

### Failure behaviour

Every cache error remains non-fatal (ADR-0001). A failed **reconcile** leaves
cached data on screen and the footer says so — "Offline — showing data from 14m
ago" in place of "Updated 14m ago · refreshing…". A failure never overwrites
the cache. Distinguishing offline from auth-rejected is Phase 5's FR-3.3.2 and
is deliberately not built here.

## Consequences

- **Startup becomes instant on a warm cache**, and the background reconcile of
  the active tab drops from ~6s to ~1.4s regardless.
- **The Usenet and Web-DL tabs finally load without `r`**, as a side effect of
  lazy reconcile. This fixes a pre-existing gap rather than adding scope.
- **`torbox.cache_ttl_minutes` is read for the first time since v0.1.0.**
- **Every release costs one cold start** because of version keying.
- **The CLI stays uncached.** `tt-wrapper list` keeps fetching fresh; a
  command-line listing that silently printed cached data would be surprising.
  The CLI does *write*-side work only through `auth` and `cache clear`.
- **The footer's relative time is wall-clock dependent**, like the resume
  prompt's. It is tested with a controlled clock, never a golden file.

## Alternatives considered

- **Only flip startup to `bypass_cache=false`, no disk cache.** Rejected by the
  maintainer: startup would still take ~1.4s, and Phase 3's other purpose —
  persistence for Phases 4/5, multi-file resume and ID-lookup memos — would
  remain unmet.
- **Inline orchestration in `app.go`.** Rejected: spreads cache logic across
  five places in a file already scheduled for splitting, ~150–200 lines.
- **Do not cache playback.** Rejected: ◐ badges would pop in a second late, and
  Enter inside that window would play from the beginning.
- **Cache playback and accept staleness.** Rejected: a stale position is
  presented as plausible and fresh.
- **Namespace the cache per account instead of clearing on re-pair.**
  Rejected: old accounts' directories would accumulate forever, and rotating a
  TorBox key would orphan the user's own cache.
- **A hand-bumped schema version.** Rejected: will be forgotten.

## Amendments from the whole-branch review (2026-09-26)

The final review of the implemented branch confirmed four interaction bugs with
throwaway tests. None was visible to a single task's review. Each changed a
decision above.

**Account isolation is by fingerprint, not only by clearing on re-pair.**
Clearing on `auth` does not cover `tt-wrapper -c other.toml`, the
`TORBOX_API_KEY` override, or a TUI still running on the old key after
`auth torbox` elsewhere. The envelope version is now
`cache.AccountVersion(config.Version, "torbox:"+apiKey)` — the release plus a
64-bit truncated SHA-256 of the account. A mismatch is a miss, exactly like a
release change. This keeps the reasons per-account directories were rejected:
filenames are unchanged, nothing accumulates, and a key rotation costs one cold
start. The hash is one-way over a high-entropy key, so FR-3.1.3 still holds.
**Trakt is not fingerprinted** — it has no stable local identity, since tokens
change on refresh. That is an accepted risk, bounded by always-reconcile plus
the launch hold to about a second of another account's badges.

**The footer tells "cached" apart from "refreshing".** `cachedAt` alone meant
"showing cached data" and could not also say whether a fetch was running, so a
tab skipped by TTL claimed to refresh all session. An `inFlight` bitmask now
drives "· refreshing…".

**A failed Trakt reconcile is said out loud where it matters.** The deferral
replays against the cached position when Trakt fails, but nothing said so. The
resume prompt now adds "Trakt is unreachable — this position may be out of
date."

**Deletes act on a snapshot.** Warm start makes a reconcile landing between `d`
and `y` routine, and `y` deleted whatever sat at the cursor index. The modal now
snapshots the item it was opened on.

**Deferred as one follow-up:** a generation counter on Trakt and per-tab fetches.
It would close a narrow in-app re-pair race (an in-flight fetch for the old
account landing after the invalidate) and let a later-finishing but
earlier-requested write win. Both self-heal on the next fetch.

Also deferred:
- A held launch can be released into a modal opened within the ~1s hold.
- The TTL is read from two places: `cfg` and the `Store`.
- Two pre-existing bugs outside this phase: `cfg.Save` can revert a concurrent
  API-key change, and `onTokenRefreshed` mutates `cfg` without synchronisation.

The warm start was measured at ~350ms (SPEC FR-3.1.2). ~295ms of it is
`matcher.ParseMedia`'s per-file regex parsing, which the cold path pays too; it
is the next thing between this and sub-50ms.

## Follow-ups resolved (2026-09-26, branch `fix/phase3-followups`, issues #2–#7)

- **Stale results (#2):** per-source fetch generations. A stale result is
  dropped before it touches the model or the disk. Cache writes moved out of the
  fetch commands into a command returned by the accepted handler.
- **Held launch behind a dialog (#3):** it stays held and is released when the
  dialog closes, via a post-update check.
- **TTL (#4):** `pkg/cache` no longer knows about lifetimes; `showTab` owns the
  policy.
- **Config writes (#5, #6):** every token or key write goes through
  `Config.persist` — re-read the file without env overrides, change one field,
  save. No callback mutates the shared `cfg`. `tt-wrapper auth` follows the same
  path, so env-set secrets never reach the config file. The Trakt fetch reads
  auth state on the update loop.
- **Parsing (#7):** a per-`Matcher` parse memo and cheaper `ParseMedia`. The warm
  first frame went from ~350ms to ~150ms.
- **Accepted:** a cache write can be lost if the TUI quits in the ~ms before its
  command runs (atomic rename; the next launch reconciles). Tags glued to an
  episode marker (`Name.1080pS01E02`) may stay in `CleanTitle`.

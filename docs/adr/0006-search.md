# ADR-0006: search through the user's own Prowlarr, titles first, with cached badges

- **Status:** Accepted and shipped in **v0.6.0** on 2026-09-29 (PR #17).
  Brainstormed and approved section by section on 2026-09-28.
- **Date:** 2026-09-28
- **Affects:** new `pkg/search`, `pkg/search/prowlarr`; `pkg/torbox` (new
  `CheckCached`); `pkg/trakt` (new `SearchTitles`); `pkg/config` (new
  `[search]`); new `internal/tui/search.go`, plus `app.go`, `types.go`,
  `modal_help.go`; new `internal/cli/search.go`; `README.md`
- **Implements:** FR-3.5.1 – FR-3.5.9 (SPEC §3.5)
- **Release:** v0.6.0. A new feature with new config is a minor bump. It isn't
  Phase 6: v1.0.0 is still the TUI redesign plus the stability contract.
- **Related:** ROADMAP §4.2 "Search — remote, not local" and its 2026-09-28
  update. The deferred refinements are in the TASKS backlog.

## Context

### The loop search closes

Today, getting something new means leaving the tool, finding a magnet in a
browser, copying it, coming back, pressing `a` and pasting. Search replaces
that with: type a title → pick the work → pick a release with its cached badge
→ `Enter` adds it → land on its library row → stream.

### Where results can come from (checked 2026-09-28)

- **TorBox's main API has no search.** `api.torbox.app/openapi.json` has
  `checkcached` (for torrents, usenet and web downloads) and `torrentinfo`, but
  no search endpoint. The ROADMAP note about `GET /torrents/search` described a
  separate host.
- **TorBox's search host is gone.** `search-api.torbox.app` (paths like
  `/torrents/search/{q}` and `/torrents/imdb:tt…`, with `check_cache`,
  `check_owned`, `metadata`, `season` and `episode`) is still called by
  AIOStreams, stremthru, comet, MediaFusion, nzbhydra2, debridui and TorBox's
  own `torbox-media-center`. Cloudflare, which is authoritative for `torbox.app`,
  answers NOERROR for it with no A, AAAA or CNAME record. No announcement was
  found. It may come back.
- **Debrid Media Manager** (AGPL-3.0, active) keeps an IMDb-keyed hash database,
  but its API needs an HMAC token that only its own server mints
  (`DMM_PROBLEM_SECRET`, 300s), plants canary IDs to catch enumeration and
  rate-limits by IP. Using it would mean getting around protections the author
  added on purpose. Ideas only: its `maxSize` and `onlyTrusted` filters are prior
  art for the deferred saved preferences.
- **Stremio addons and AIOStreams.** Their protocol is trivial, but public
  instances need the TorBox API key inside the addon URL. Self-hosting
  AIOStreams needs Docker and Node. Out.
- **Prowlarr**, run by the user, aggregates their indexers. The maintainer
  installed it on 2026-09-28: the CachyOS package, v2.6.5.5623, running as a
  system service under its own `prowlarr` user with data in `/var/lib/prowlarr`.

### What Prowlarr actually does (read from the v2.6.5.5623 source)

- **Torznab is per indexer only** (`/{id}/api`, `/api/v1/indexer/{id}/newznab`).
  There is no combined Torznab endpoint.
- **`GET /api/v1/search` searches every enabled indexer in one call** and
  returns JSON `ReleaseResource`s: `guid`, `title`, `size`, `seeders`,
  `leechers`, `infoHash`, `indexer`, `indexerId`, `publishDate`, `protocol`,
  `imdbId` and others. Parameters: `query`, `type`, `indexerIds`, `categories`,
  `limit`, `offset`.
- **Auth:** the `X-Api-Key` header, or the `apikey` query parameter
  (`AuthenticationBuilderExtensions.cs`). An unauthenticated request gets 401.
- **ID searches go inside `query` as tokens**, parsed by
  `NewznabRequest.QueryToParams`:
  - movies: `{ImdbId:tt…}`, `{TmdbId:}`, `{TraktId:}`, `{Year:}`
  - TV: `{ImdbId:}`, `{TvdbId:}`, `{TmdbId:}`, `{Season:}`, `{Episode:}`
  - tokens are case-insensitive and removed from the free text
- **An indexer that lacks any ID parameter we send is skipped entirely**
  (`HttpIndexerBase.Fetch`: "search skipped due to unsupported capabilities").
  Parameters are ANDed, so sending more IDs means fewer indexers. The ID search
  therefore sends the IMDb ID alone (plus season and episode for TV), and a text
  search runs alongside it to reach the indexers that were skipped.
- **Indexer failures are swallowed.** `DispatchIndexer` logs and returns an
  empty list, so one broken indexer looks like "no results" from that indexer.
  The whole call only fails for things like a bad key.
- **`indexerIds=-2` means every torrent indexer** (`-1` means every usenet
  indexer). A single id searches just that indexer. `categories` narrows
  indexers and results: 2000 is Movies, 5000 is TV.
- **A combined search waits for the slowest indexer.** On the maintainer's
  indexers the combined TV search took 40s and the combined movie search passed
  60s without an answer. Asking each indexer separately returns the fast ones in
  under 3s (see "Measured" below).
- **`GET /api/v1/indexer`** lists the configured indexers with `enable`,
  `protocol` and `capabilities.movieSearchParams` and `tvSearchParams`.
- **Prowlarr's de-duplication is by `guid`, not infohash**, so the same release
  from two indexers comes back twice.
- **Free-text results are already filtered by term** (`FilterReleasesByQuery`):
  with more than one search term, a result must contain at least two of them.
- **`magnetUrl` and `downloadUrl` are rewritten to Prowlarr proxy links** of
  the form `{server}{urlBase}/{indexerId}/download?apikey=<key>&link=…&file=…`
  (`DownloadMappingService.ConvertToProxyLink`).
- **That download endpoint answers 301 to the magnet** when the indexer's link
  is a magnet (`RedirectPermanent(magnetUrl)`). It accepts the `X-Api-Key`
  header in place of the `apikey` parameter. Tested on 2026-09-28: LimeTorrents
  and TorrentDownload rows resolved to `magnet:?xt=urn:btih:…` in 1–2s. The
  first one resolved duplicated a Knaben result that did carry a hash.
- **`infoHash` is filled when the indexer gives one or gives a magnet in the
  listing.** Cardigann derives it from the magnet's `xt`, which can be base32.
  Indexers that only link to a details page or a download (LimeTorrents,
  TorrentDownload) leave it empty.

### Trakt title search (checked 2026-09-28, live, read-only)

`/search/movie,show` needs only the client ID. It corrected `intersteller` →
Interstellar (2014) and `breking bad` → Breaking Bad (2008). For `dune` it
listed every distinct work. It can't fix a typo that is itself a real word:
`the dark night` returns other films. `pkg/trakt` already has `SearchMovies`
and `SearchShows` (unauthenticated), which the matcher uses.

## Decision

### 1. Prowlarr's native search API, behind a `Searcher` interface

- **`pkg/search`** has no Prowlarr knowledge. It defines `Query`, `Result`,
  `Searcher`, `ParseQuery`, `Merge`, `Sort` and infohash and magnet handling.
- **`pkg/search/prowlarr`** is the only backend, and calls `GET /api/v1/search`
  with `X-Api-Key`.
  - A Torznab backend (for Jackett), a TorBox-search backend or usenet can be
    added later as siblings behind the same interface.
  - This is a search-source interface, not an alternative debrid backend, so it
    doesn't conflict with the SPEC §5 non-goal.
- **One request per indexer.** The backend lists indexers with
  `GET /api/v1/indexer`, keeps those with `enable` true and `protocol` torrent,
  and sends one `GET /api/v1/search` per indexer (`indexerIds=<id>`), all in
  parallel.
  - Each indexer has its own 60-second timeout. A slow indexer delays only its
    own rows.
  - Results stream in: each indexer's reply is merged, badge-checked and shown
    as it arrives.
  - A status line shows every indexer's state, for example
    `Knaben ✓ 4 · LimeTorrents … · YTS ✗`. This also surfaces the failures that
    Prowlarr would otherwise swallow.
  - The indexer list is fetched once per search, so enabling or disabling an
    indexer in Prowlarr takes effect on the next search without a restart.
- **Torrents only in v0.6.0.** Usenet later is `protocol` usenet, plus TorBox's
  usenet add path. The response's `protocol` field already tells the two apart.
- **The magnet is built from `infoHash`** as
  `magnet:?xt=urn:btih:<hex>&dn=<title>`. `magnetUrl` is never decoded.
- **Infohashes are normalised** to lowercase 40-character hex. Base32 is
  decoded, and anything else is treated as missing.

### 1a. Results without an infohash are resolved through Prowlarr

This reverses the earlier "drop and count" ruling. Two of the maintainer's five
general indexers (LimeTorrents and TorrentDownload) never send a hash, and they
carry most of the fresh regional releases (see "Regional releases" below).
Dropping them would drop that whole catalogue.

- A result with no valid infohash but a `downloadUrl` becomes a **pending** row,
  with the badge `…`.
- **A background resolver** takes pending rows in descending seeder order, 2 at
  a time and at most 20 per search. For each one it asks Prowlarr's download
  link for the redirect, reads the `magnet:` in the `Location` header, extracts
  the hash, and hands the row to `Merge` and `CheckCached` like any other.
  - The 10 highest-seeded pending rows are resolved straight away. The other 10
    are resolved when the cursor reaches them.
  - Pending rows beyond the 20 stay `…` until `Enter`.
  - The CLI resolves the first 10 before printing.
- **`Enter` on a row that is still pending** resolves that row first, then adds
  it. This is allowed past the budget of 20, since it's one deliberate request.
- **A resolve is not a local lookup.** Prowlarr fetches the page from the
  indexer's site (`DownloadService.DownloadReport`), records the fetch as a
  grab in its history, and counts it toward that indexer's Grab Limit. It also
  keeps at least 2 seconds between requests to one indexer
  (`HttpIndexerBase.RateLimit`, or longer if the definition sets
  `requestDelay`). That is why concurrency and the budget are
  kept small.
- **A 429 from the download link stops that indexer's resolves** for the rest of
  the search. Prowlarr returns it when the Grab Limit is reached or the indexer
  is disabled after failures. Its rows stay `…`, and the status line entry
  says "limited".
- A row that fails to resolve, or whose download answers with a `.torrent` body
  instead of a redirect, is dropped and counted ("2 results couldn't be
  resolved"). Hashing `.torrent` files needs a bencode parser and is deferred.
- **Guard rails**, because the link is a key-bearing URL:
  - The `apikey` parameter is removed while the JSON is decoded, before the link
    is stored anywhere. The key goes in the `X-Api-Key` header instead.
  - The link is used only if its scheme, host and port equal the configured
    `prowlarr_url`, and its path is `{urlBase}/{n}/download`. Anything else
    counts as unresolvable.
  - The HTTP client never follows redirects (`CheckRedirect` returns
    `http.ErrUseLastResponse`). Only a 301 or 302 whose `Location` starts with
    `magnet:` is accepted, and nothing else is read from the response body.
  - The link lives only in memory on the backend's own result value. It is never
    logged, printed, put in an error or written to `--json`.

### 2. Two steps: pick the work, then the release

- **Step 1, titles.**
  - Free text goes to a new `trakt.SearchTitles` (`/search/movie,show`, Trakt's
    order kept, up to 10 hits).
  - A typed IMDb ID (`tt0903747`, `tt0903747 S02E05`, `tt0903747 S02`) skips
    this step.
  - `ctrl+r` (the CLI's `--raw`) sends the raw text to Prowlarr as a plain
    search, with no Trakt step and no title filter.
  - Choosing a show asks for `S02`, `S02E05` or blank (the whole show), which
    saves a call to fetch the list of seasons.
- **Step 2, releases.** Each indexer gets up to two requests, all in parallel:
  - **ID**, only to indexers whose capabilities list `imdbId` for that search
    type: `type=movie`, `query={ImdbId:tt…}`, `categories=2000`; or
    `type=tvsearch`, `query={ImdbId:tt…}{Season:n}{Episode:m}`,
    `categories=5000`. On the maintainer's indexers only YTS qualifies, and only
    for movies.
  - **Text**, to every indexer: `type=search` with `"Title Year"` for a movie,
    `"Title S02E05"` or `"Title S02"` for TV, and `"Title"` for a whole show.
    Same categories.
  - Text results are kept only if `matcher.ParseMedia` of the release title
    matches the chosen work's title after normalisation. A movie's year must be
    within ±1, since releases are sometimes a year off. An episode search also
    keeps season packs for that season, tagged `pack`.
  - The match must tolerate a short leftover fragment before the title. Some
    indexers pass on uploads whose `www.site.tld -` prefix has already been cut
    down to something like `pany  Title (2026) …`, which `stripSiteMarkers` no
    longer recognises. The rule is that the normalised title appears as a whole
    run of words ending just before the year or episode marker, not that the
    release name starts with it.
  - `Merge` combines both sets by infohash, keeping the highest seeders and the
    list of indexers. New hashes go to the badge checker (§6a). The default sort
    is cached first, then seeders.
- **The library shortcut:** `S` on a Trakt-matched library row starts step 2
  directly with that row's IMDb ID, season and episode. On a row with no match
  it says so.

### 3. Picking a release is manual in v0.6.0

- Each row shows:
  - the cached badge (`●` cached, `○` not, `■` in your library, `?` unknown,
    `…` still resolving)
  - resolution, source, codec, HDR, audio and release group, all from the
    existing `matcher.ParsedMedia`, so there is no second parser
  - size, seeders, and the sanitised release name
- `/` filters the results. `O` cycles the sort: cached first, size, seeders,
  resolution. (`o` already opens the file tree in the library, and the same key
  shouldn't mean two things.)
- Nothing is saved. Saved preferences and auto-pick are deferred: TASKS
  backlog, ROADMAP §4.2.

### 4. `Enter` adds, then jumps to the row

- `CreateTorrent` with the built magnet. On success, switch to the Torrents tab,
  refetch bypassing the cache, and put the cursor on the returned `TorrentID`.
- If that ID isn't in the list yet, the status line says "Added, waiting for
  TorBox", and the cursor moves to the row when it appears on a later refresh.

### 5. A CLI command for the same flow

- `tt-wrapper search <query>` prints title hits, with IMDb IDs, for free text.
- For an ID query it prints the releases table.
- It takes `--raw`, `--json` and `--sort`. It never adds; the output gives the
  magnet for `tt-wrapper add`.

### 6. Config: `[search]`

- `prowlarr_url` (for example `http://127.0.0.1:9696`, including any Prowlarr
  URL base) and `prowlarr_api_key`, with a `PROWLARR_API_KEY` env override like
  the other keys.
- **Search is off while either is empty,** and every entry point says how to
  turn it on.
- **Plain `http` is allowed only to a loopback host** (`localhost`, `127.0.0.0/8`,
  `::1`). Any other `http` URL is rejected. This is checked when search is used,
  not at config load, so a bad `[search]` block can't stop the rest of the app
  from starting. This is a ruling on the approved section 3, which said "the
  config fails to load".

### 6a. Caching and rate limits

**In memory only; no database.** A stored cached flag goes stale as TorBox
evicts content, and a table of hashes and release names on disk would amount to
a search history. Search keeps its "nothing written to disk" rule.

- **Cached-status cache.** Each hash keeps its `CheckCached` answer for 15
  minutes, for as long as the TUI runs. Repeating a search, stepping back, or
  searching again from the library costs no new TorBox calls while the answer
  is fresh. The CLI is one-shot and doesn't cache.
- **Resolved-hash cache.** A resolved row's hash is remembered for the session,
  keyed by `indexerId` and `guid` (not by the download link), so it's never
  resolved twice.
- **Library hashes.** A hash already in the loaded TorBox list shows the badge
  `■` ("in your library") without a check. `Enter` on it jumps to the existing
  row and doesn't add again.

**One badge checker for the whole app**, so every search shares one budget and
TorBox sees an even trickle:
- Hashes that aren't in either cache go into a queue. A batch is sent when 100
  are waiting, or 2 seconds after the first one arrived, whichever comes first.
- One request at a time, and at least 2 seconds between the starts of two
  requests (at most 30 a minute).
- On a 429 it waits for `Retry-After` or else 30 seconds, doubling on each
  further 429 up to 5 minutes, and resets after a success. Rows stay `?` while
  it waits, and the status line says "TorBox rate limit, retrying in 1m".
- On any other failure the batch's rows show `?` and the checker carries on with
  the next batch. A failed batch isn't retried; searching again re-queues it.
- A search that is cancelled or replaced removes its not-yet-sent hashes from
  the queue.
- `torbox.CheckCached` makes one attempt, with no retry inside the client. On a
  429 it returns an error that carries `Retry-After`. The existing client
  retries 429s by itself and doesn't expose the header, so this call gets its
  own path.
- For a typical popular title (about 200 unique hashes across the maintainer's
  indexers), that is 3–5 `checkcached` calls, spread over the time results
  arrive.

**No search-as-you-type.** Trakt and Prowlarr are called only when `Enter` is
pressed.

### 7. Errors and secrets

- **Each error shows on the screen it belongs to:**
  - not configured
  - Prowlarr unreachable ("can't reach Prowlarr at <host:port>. Is it running?")
  - 401 ("Prowlarr rejected the API key")
  - an indexer failed or timed out (its entry on the status line shows `✗`, and
    every other indexer's results still show)
  - no results (with a hint to try `ctrl+r`)
  - Trakt search failed (`ctrl+r` is still offered)
  - `CheckCached` failed (badges show `?`) or rate-limited (badges show `?`,
    with the wait on the status line)
  - `CreateTorrent` failed (the existing `classifyFetchErr` wording)
- **Cancellation and stale replies:**
  - Every per-indexer request has a 60-second timeout, and each resolve has
    10 seconds. Titles and `CheckCached` keep the existing client timeouts.
  - `Esc` cancels what's in flight.
  - A newer search cancels the older one, and replies carry a sequence number so
    a late reply is ignored.
- **The Prowlarr key never appears in a log line, an error, `--json` output or
  the config's `String()`** (it goes through `MaskSecret`). Errors built from a
  URL pass through `logging.Redact`, which already blanks `apikey=`. Download
  links are covered by the guard rails in 1a.
- **Release names are untrusted.** They're passed through
  `matcher.SanitizeDisplay` before any TUI or CLI output.
- **Nothing new is written to disk.**

## Consequences

- Search needs a Prowlarr the user runs and configures. The README gets a
  "Search (optional)" section. Without it the app is unchanged.
- **ID searches only reach indexers with IMDb support.** The parallel text
  search covers the rest, at the cost of the title filter occasionally keeping a
  wrong release or dropping a right one with an unusual name.
- Hash-only magnets carry no trackers. For cached items that doesn't matter.
  Uncached ones rely on TorBox finding peers through DHT, which can make those
  downloads slower to start.
- **Hashless results cost extra requests:** up to 20 resolves per search, 2 at
  a time, each of which Prowlarr records as a grab. Rows past the first 20 by
  seeders stay pending until `Enter`.
- **Badges fill in over tens of seconds, not instantly,** because the badge
  checker paces TorBox requests. That's a deliberate trade for never sending a
  burst.
- **Results that resolve to a `.torrent` file are skipped** and counted.
- **A search makes several requests to Prowlarr** (one or two per indexer), in
  exchange for fast indexers answering in seconds.
- `pkg/trakt` gains a third search method. It shares the existing
  unauthenticated `search` helper.

## Measured on the maintainer's Prowlarr (2026-09-28)

Per indexer, text search, `Interstellar 2014` / `Breaking Bad S02E05`:

| Indexer | Movie | TV | Hashes |
| --- | --- | --- | --- |
| The Pirate Bay | 97 in 0.6s | 10 in 2.8s | yes |
| YTS | 5 in 0.5s (IMDb ID search: 3 exact in 1s) | none (movies only) | yes |
| Knaben | 76 in 52s | 4 in 12s | yes |
| TorrentDownload | 50 in 0.6s | 14 in 2.4s | no, resolved via 1a |
| LimeTorrents | 40 in 17s | 12 in 22s | no, resolved via 1a |
| nekoBT, Nyaa.si | 0 | 0 | anime-only |

Combined (`indexerIds=-2`): TV took 40s; the movie search passed 60s.

## Maintainer's Prowlarr (state on 2026-09-28)

The problems found at install are fixed:
- **Bind Address** is `127.0.0.1`. The web UI hides it under "Show Advanced",
  and saving from the UI didn't stick, so it was set through
  `PUT /api/v1/config/host/1` (the GET body resent with only `bindAddress`
  changed, which keeps the password hash) and Prowlarr was restarted through
  `POST /api/v1/system/restart`. Verified: it listens on `127.0.0.1:9696` only,
  and a LAN connection is refused.
- **Authentication** is Forms, Enabled. URL Base is empty.
- **Log level** is Info and analytics are off.
- **`config.xml`** is mode 640.

Indexers, taken from MediaFusion's `resources/json/prowlarr-indexers.json` where
Prowlarr has them: LimeTorrents, nekoBT, Knaben, The Pirate Bay,
TorrentDownload, YTS and Nyaa.si. Left out:
- EZTV, which fails with "blocked by CloudFlare Protection" and would need
  FlareSolverr.
- Torrent Downloads, deleted.
- BitSearch, TheRARBG, Torlock, YourBittorrent, Badass Torrents, iTorrent and
  GkTorrent, which aren't in Prowlarr v2.6.5.5623's definitions.

## Regional releases (Tamil, Telugu, Malayalam)

MediaFusion's regional catalogue comes from its own scraper, not from Prowlarr:
`backend/src/jobs/handlers/spiders/tamil_forums.rs` reads the Invision forum
listings of TamilMV and TamilBlasters (`/index.php?/forums/forum/<id>/page/<n>/`)
with CSS selectors. There is no public Prowlarr or Jackett definition for
either forum. Prowlarr's only Indian indexers (BwTorrents, DesiTorrents,
TmGHuB, DesiGaane) are private trackers.

The existing indexers already carry those uploads. A `"<language> 2026"` text
search on 2026-09-28 returned, for Tamil: LimeTorrents 40, TorrentDownload 50,
The Pirate Bay 4 and Knaben 3. Telugu and Malayalam gave similar numbers. Most
were published in the previous two days, and many carry the forum's name as a
cut-down prefix. Two consequences:
- **Resolving hashless results (1a) is what makes regional search work.** The
  fresh releases are nearly all on LimeTorrents and TorrentDownload.
- **The title filter must tolerate the leftover prefix** (see step 2).

Out of scope for v0.6.0: a forum scraper, whether as a custom Cardigann
definition in `/var/lib/prowlarr/Definitions/Custom` or as a second `Searcher`
backend. Forum HTML changes without notice, and the indexers above already
relay the same uploads within a day or two.

## After release

- Built in 11 commits. Three commits were added on top:
  - Trakt titles and IDs go through `matcher.SanitizeDisplay` in the CLI and
    TUI.
  - `Config.String()` masks the Prowlarr URL down to scheme, host and path
    (`maskURL`).
  - The library footer lists `[s] Search`, and `fitHints` drops hints in a set
    order (Tab, Trakt, Files, Delete, Filter, Add, Search, Stream) instead of
    wrapping. Help and Quit always stay. This removes the footer wrap at 120
    columns with a long status, part of FR-4.1.1 ahead of Phase 6.2.
- **Security review, 2026-09-29:** nothing found at confidence 8 or above.
- **Manual check, 2026-09-29** on the maintainer's Prowlarr and TorBox with
  `--verbose`:
  - Title search, add, jump and stream all worked.
  - `checkcached` calls were about 2.0s apart.
  - Prowlarr logged one request per indexer per search.
  - No key appeared in the debug log, and the mpv args carried only the proxy
    host.
  - TorrentDownload was still being searched, so it was not really deleted on
    2026-09-28.
- **Not covered by the debug log:** the Prowlarr client logs nothing. Resolve
  pacing and 429s are visible only in Prowlarr's own log, which at Info level
  doesn't show the download redirects. Adding redacted debug lines is a
  candidate follow-up.
- **The README drops the `search` and `stream` CLI examples** at the
  maintainer's request, because of how they read in a public repo. The
  commands are unchanged.

## Open questions

- **TorBox's rate limits aren't published anywhere we could read.** The docs
  pages render only in a browser, and the OpenAPI spec names a limit for one
  notifications endpoint only. One live `checkcached` reply carried
  `x-ratelimit-limit: 300` per 60s. The pacing here (one request at a time, 2s
  apart, so at most 30 a minute) stays at a tenth of that on purpose, because
  the limit may be shared with the rest of the app and may differ per plan.
- **`checkcached` accepted 100 hashes in one POST** in the live probe. Whether
  it takes more is untested and not needed. The batch size, spacing and cache
  lifetime are constants.
- **How well the title filter does on real results,** including regional
  titles with a leftover prefix. It will be measured at the end-to-end check on
  the maintainer's indexers, and loosened if it drops too much.
- **Whether 20 resolves per search (10 up front) is the right budget.** Both are
  constants.
- **If `search-api.torbox.app` comes back,** a TorBox backend would give cached
  flags directly, with no separate `CheckCached` call.

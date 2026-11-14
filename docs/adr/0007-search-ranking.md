# ADR-0007: rank and cap search results before checking them, grouped by resolution

- **Status:** Accepted and shipped in **v0.7.0** on 2026-11-14 (PR #18),
  amended before planning (see the end). Brainstormed and approved section by
  section on 2026-09-29.
- **Date:** 2026-09-29
- **Affects:** `pkg/search` (new `lang.go`; `rank.go` rewritten; `resolver.go`
  order), `pkg/search/prowlarr` (debug logging); `pkg/trakt` (`languages` on
  title hits and ID lookups); `pkg/config` (`preferred_groups`,
  `[search.resolutions]`); `internal/tui/search.go`; `internal/cli/search.go`;
  `README.md`
- **Implements:** FR-3.6.1 – FR-3.6.10 (SPEC §3.6). Supersedes
  parts of FR-3.5.2, FR-3.5.5, FR-3.5.7 and FR-3.5.8, listed in §3.6.
- **Release:** v0.7.0. Changed behaviour and new config make it a minor bump.
  It isn't Phase 6.
- **Related:** ADR-0006 (search), and its open question on how much the title
  filter drops. The backlog item
  "Saved release preferences for search" in TASKS is partly
  covered here (see Consequences).

## Context

### What the first real use of v0.6.0 showed (2026-09-29)

- **The list looks jumbled.** The default sort is cached first, then seeders.
  Resolution plays no part, so 480p rows sit above 4K ones and 1080p and 4K
  rows are mixed together.
- **The list moves while it is being read.** `sortReleases` runs whenever an
  indexer answers, a resolve finishes or a badge batch lands
  (`internal/tui/search.go:367`, 437, 510). The cursor stays on its row, but
  the rows around it move.
- **Everything gets checked.** Prowlarr is asked for `limit=100` per indexer,
  with 6–7 indexers, so there are up to 700 rows before merging. Every hash is
  queued for `checkcached`. One title search took 9 batches, about 2s apart,
  so the badges took about 20s to settle.
- **Most of those rows are no use.** The maintainer watches 4K and 1080p and
  keeps 720p for rare finds, and has no use for anything lower. About ten good
  4K options are plenty.

### What the maintainer wants from a result list

Taken from the brainstorm answers:

- **Cached matters, but it doesn't decide.** Mainstream releases are usually
  cached. Regional ones often aren't, and are still wanted.
- **Resolutions:** 4K and 1080p, plus 720p until usenet is set up. Nothing
  lower.
- **Quality:** a known encoder's 4K HEVC is better than an anonymous upload of
  the same film. The group names are the maintainer's choice, so they go in
  config and never in the repo.
- **Language:** English for English films, and the original audio for
  regional films (Tamil, Telugu, Malayalam and others). Release names tag
  language unevenly, so this is a preference, not a filter.

### What Trakt offers for language (checked 2026-09-29)

From the API blueprint (`jsapi.apiary.io/apis/trakt.apib`, read-only):

- A movie at `?extended=full` carries `"languages": ["en"]`, `"country"` and
  `original_title`.
- A show at `?extended=full` carries `"languages": ["en"]` as well.
- Text search (`/search/{type}?query=`) and ID lookup (`/search/imdb/{id}`)
  both accept `extended=full`. The language comes with the request that
  already happens, so it costs nothing extra.
- The codes are ISO 639-1: `en`, `ta`, `te`, `ml`, `hi`, `kn`.

## Decision

### 1. Group by resolution, drop the junk, rank without weights, then cap

All of this lives in `pkg/search` and is shared by the TUI and the CLI.

- **Groups,** always shown highest first:

  | Group | Parsed resolutions | Default cap |
  |---|---|---|
  | 4K | `2160p`, `4k`, `uhd` | 10 |
  | 1080p | `1080p`, `1080i` | 10 |
  | 720p | `720p` | 5 |
  | 576p | `576p` | off |
  | 480p | `480p` | off |
  | Unknown | none parsed | 5 |

- **Hard drops,** applied before ranking:
  - 0 seeders.
  - CAM, TS, TC, HDCAM and SCR (screener) sources.
  - Implausibly small files for the resolution, such as a "4K" film under
    1 GB. A size of 0 means unknown and is never dropped. The floors are in
    SPEC FR-3.6.2.
  - A row already in the library is never dropped. Its hash is known, so it
    doesn't need checking or resolving.
- **Rank inside a group** as a plain ordering. Each key only breaks ties in
  the ones before it:
  1. **Language.** The work's original language from Trakt ranks first, then
     multi or dual audio, then untagged, then any other language. For an
     English work, an untagged release counts as English, since English
     releases rarely say so.
  2. **Episode before pack,** in an episode search only.
  3. **Trusted group:** a release whose group is in `preferred_groups`.
  4. **Easy to stream:** HEVC or AV1 first, then every other codec, including
     unknown ones. A remux, or a file over the size ceiling for its
     resolution, sinks to the bottom but is not dropped.
  5. **Seeders,** most first.
- **Cap:** only the top N of each group are shown, and only those are checked
  with TorBox or resolved. `m` shows more.
- **Why no weights:** a points system needs tuning and is hard to explain. A
  plain ordering can be predicted by reading the row: "Tamil, group I trust,
  HEVC, 40 seeders".
- **Why cap after ranking** and not by lowering Prowlarr's `limit`: a lower
  limit trims before anything is ranked and could drop the best release.
- **Why cached status isn't a rank key:** it isn't known until after the cap
  is chosen, and waiting for it is the 20s this ADR removes. `c` covers "just
  show me what streams now".

### 1a. Language detection

- `lang.go` reads language tags from the release name. It looks only at the
  part after the year or episode marker (the whole name when there is none),
  and matches whole words, case-insensitively:
  - Tamil, Telugu, Malayalam, Hindi and Kannada, by full name or three-letter
    code (`tam`, `tel`, `mal`, `hin`, `kan`)
  - English (`english`, `eng`)
  - multi or dual audio (`multi`, `dual`, `dual audio`)
- Two or more different languages in one name count as multi.
- Anything else is untagged. This is a small, fixed list on purpose: it covers
  the maintainer's regional indexers, and a wrong guess only moves a row
  within its group.
- A raw search has no chosen work and no year marker. The language is still
  detected from the whole name, for display only.

### 2. The TUI settles once, then fills badges in place

- **Settling:**
  - While indexers answer, the screen shows a live counter,
    `Searching… 5 of 7 indexers`, and no rows.
  - The list is ranked, capped and drawn once, when every indexer has
    answered or 5s after the first answer, whichever comes first.
  - The shown rows go to the checker in one go. With the default caps that is
    at most 30 hashes, so one `checkcached` request.
- **Nothing reorders after that.** Badges and resolved hashes fill in place.
  This removes every re-sort on arrival.
- **Late results:** an indexer that answers after the list is drawn doesn't
  move existing rows. Its new rows go into a "Late results" section at the
  bottom. They are dropped and ranked the same way, capped at 10, and checked
  in their own batch.
  A late row with the same hash as a shown row only adds its indexer name to
  that row.
- **Layout:**
  - Headers like `── 4K · 10 of 34 ──` replace the resolution column. The
    cursor skips headers.
  - A language column is added. It is blank when untagged, and uses short
    labels (`TAM`, `TEL`, `MAL`, `HIN`, `KAN`, `ENG`, `MULTI`) to keep rows
    inside 120 columns.
  - Late results show each row's resolution next to its source, since they
    aren't under a resolution header.
- **Keys:**
  - `c` toggles cached only.
  - `m` shows the next N in the cursor's group, which are checked then.
  - `O` sorts inside groups by rank (the default), size or seeders.
  - `/` filters across groups, and the headers show filtered counts.
  - `Enter`, `s`, `S` and `Esc` are unchanged.
- **The footer fits the width,** like the library footer (`fitHints`), and no
  longer wraps.
- **The `cached` and `resolution` sorts go away.** Grouping already does the
  resolution sort, and cached is `c`. The release notes and the help modal
  must say that `O` changed.

### 3. Config, Trakt, CLI and the debug log

- **Config:**

  ```toml
  [search]
  preferred_groups = []

  [search.resolutions]
  "2160p" = 10
  "1080p" = 10
  "720p" = 5
  unknown = 5
  ```

  - `preferred_groups` is empty by default. The repo, its tests and its docs
    never name a real group.
  - A resolution missing from the table is dropped. Listing `"480p" = 5`
    brings that group back. A cap of 0 drops the group, and the most a cap
    can be is 50.
  - An unknown key, a negative cap or a cap over 50 is an error that names the
    key. As with the Prowlarr URL (FR-3.5.1), it is reported when search is
    used and never stops the app starting.
  - Leaving the whole table out gives the defaults above. Having a table with
    only some keys means only those groups are shown, since the table is the
    list.
  - The size floors and ceilings are constants, not config.
  - `tt-wrapper config` shows the new fields.
- **Trakt:**
  - The title search adds `extended=full`. It is still one request, and each
    hit carries `languages`.
  - `S` on a library row, or a typed IMDb ID, makes one lookup,
    `/search/imdb/{id}?extended=full`, for the language. It runs alongside
    the indexer requests, so it adds no wait.
  - If Trakt is down, not configured, returns no languages, or hasn't
    answered by the time the list is drawn, the search runs without the
    language key. The language column still shows.
  - A raw search (`ctrl+r`, `--raw`) has no language key.
- **CLI (`tt-wrapper search`):**
  - It waits for every indexer (the 60s timeout still applies) instead of
    settling, then ranks, caps and checks the same way.
  - Text output is grouped under the same headers. `--json` adds `group` and
    `language` to each release.
  - `--sort rank|size|seeders`, default `rank`. `cached` and `resolution` are
    kept as synonyms for `rank`, so v0.6.0 scripts still work.
  - `--cached-only` prints only cached and in-library rows.
  - `--all` lifts every cap, so every row that survives the drops is checked,
    paced as today. The resolve budget still applies.
- **Debug log (`--verbose`):**
  - Each Prowlarr request logs its method, path and query with any `apikey`
    blanked, and its status, duration and row count. The header key is never
    logged, as today.
  - Each resolve logs the indexer, the outcome (magnet, not a magnet, 429,
    timeout, error) and the duration. The download link is never logged,
    because it carries the key.
  - Each search logs one summary line: rows in, rows merged, per-group
    shown/total counts, and how many rows each drop reason removed, including
    the title filter. No release names. This answers ADR-0006's open question
    about the title filter.
  - The README's debug-log paragraph, which says Prowlarr requests aren't
    logged, changes to match.

### 4. Scope

- **In** (numbering from `IDEAS-search-next.md`):
  - 1, 2 and 5: grouping, ranking and the cap. The cap makes the list stop
    jumping.
  - 3: resolves in rank order. Badge order no longer matters.
  - 4, 6 and 7, the last in its small form (episode before pack).
  - 8, 9 and 10.
  - 11: the doubled doc comment on `sortReleases`.
- **Out:**
  - usenet
  - a TorBox search backend
  - auto-pick
  - size limits as config
  - merging near-duplicates that have no hash
  - a GB-per-hour check from Trakt's runtime
  - Phase 6
- Item 12 (TorrentDownload still enabled) is the maintainer's Prowlarr, not
  code.

## Consequences

- **A search is quicker.** With the default caps it checks at most 30 hashes,
  in one TorBox request instead of about 9, and the badges land about 2s after
  the list is drawn, not 20s.
- **The list waits up to 5s before drawing.** A slow indexer no longer delays
  anything, and its rows show up under Late results.
- **Some releases are never seen without `m` or `--all`.** Rows past the cap
  aren't checked, so a cached release ranked 11th in its group shows only
  after `m`. The ranking is meant to make that rare.
- **A 0-seeder release is dropped even if TorBox has it cached.** Its cached
  state isn't known before the drop. The summary line counts these drops, so
  the manual check can show whether this costs anything.
- **The language key depends on Trakt.** Without it, the order inside a group
  falls back to episode, group, streaming and seeders.
- **`O` changes meaning** and the `cached` and `resolution` sorts go. The CLI
  keeps both names as synonyms.
- **Partly covers the "Saved release preferences" backlog item.**
  `preferred_groups` is its `prefer`. `max_size` stays a constant ceiling that
  only sinks rows, and `exclude` is still open.
- **More code in `pkg/search`,** all pure functions over `[]Result`, so it is
  table-tested with neutral fixtures and no I/O.

## Open questions

- **Does Trakt's `languages` list the original language first** for every
  work? The blueprint shows one-element lists only. To be checked against one
  real regional movie and one real show response before planning, with no key
  printed and no bursts.
- **Is 5s the right settle window?** It is a constant. The summary line and
  the manual check will show how many results arrive late.
- **Are the size floors and ceilings right?** They are constants, set from
  typical encodes. The drop counts in the summary line will show whether the
  floors bite.
- **Should a trusted group outrank language?** The brainstorm said no: the
  wrong audio can't be watched, whereas an unknown encoder can. Revisit if the
  maintainer finds a trusted group's release sitting below a random one in the
  right language.

## Amendments before planning (2026-11-14)

- **Original language comes from Trakt's `language` field,** falling back to
  the first entry of `languages`. `language` is the original language, so the
  first open question no longer decides anything. A real regional response
  still gets checked in the release's manual check. SPEC FR-3.6.3.
- **No second Trakt lookup.** `S` on a library row and a typed IMDb ID
  already make one `/search/imdb/{id}` lookup for the title before the
  releases step. It now asks for `extended=full` and carries the language.
  Section 3's "runs alongside the indexer requests" is dropped. SPEC FR-3.6.8.
- **A single episode is read from the parse,** not the search kind, so a raw
  search doesn't judge episodes by film floors and ceilings. SPEC FR-3.6.2.

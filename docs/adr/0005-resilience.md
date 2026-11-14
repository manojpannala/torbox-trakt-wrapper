# ADR-0005: a renewing localhost stream proxy, and failures sorted by what the user can do

- **Status:** Accepted and shipped in **v0.5.0** on 2026-09-28 (PR #16).
- **Date:** 2026-09-27
- **Affects:** new `pkg/stream`; `internal/tui/app.go`, `types.go`, new
  `fetcherr.go`; `internal/cli/stream.go`; `pkg/config/config.go`;
  `pkg/trakt/client.go` (logging only)
- **Implements:** FR-3.3.1 – FR-3.3.3 (SPEC §3.3)
- **Release:** v0.5.0. A new default network path is a minor bump, not a patch.
- **Related:** a local prototype (deleted after the plan was
  written; its findings are recorded below)

## Context

### Stream URL expiry (FR-3.3.3)

TorBox's `requestdl` link is signed and documented as valid for 3 hours. mpv
gets the signed link directly, and the default
`--stream-lavf-o=reconnect=1,reconnect_streamed=1,reconnect_delay_max=30` can
redo a dropped connection but can't re-sign it.

**Spike, 2026-09-27**, mpv 0.41 with the wrapper's own args. A local server
serves a 120s mkv and answers 403 once the link is 6s old:

| Scenario | What mpv did | `end-file` reason | Exit code |
| --- | --- | --- | --- |
| Seek past the buffer after expiry | logs `HTTP error 403`, stops | `eof` | 0 |
| Connection dropped after expiry | reconnects, gets 403, plays what it had buffered, stops | `eof` | 0 |
| Either case, through a proxy that re-resolves on 403 | plays on; one re-resolve | `quit` (user) | 0 |

**An expired link is indistinguishable from the film ending.** mpv reports
`eof` and exits 0. The monitor would scrobble Stop at whatever percent was
reached, and the user would see "Playback finished".

### Failure reporting (FR-3.3.1, FR-3.3.2)

- `LibraryFetchFailedMsg` handling treats every error the same. With a cache it
  shows "Offline — showing data from …", including when TorBox rejected the
  key. Without one it prints the raw error, which for a dial failure includes the
  full request URL.
- The header shows `Trakt: [Connected ✓]` whenever a token is stored, even after
  Trakt has rejected it. The proactive refresh in `pkg/trakt` `doRequest`
  discards its error (`_ = err`), so a revoked sign-in leaves no trace in the
  log either. A rejected refresh already reaches the TUI as `trakt.ErrUnauthorized`,
  through the reactive 401 path.

## Decision

### 1. A renewing proxy per playback (`pkg/stream`)

Approaches weighed:

1. **Detect after the fact.** Compare the `eof` position with the duration and
   offer "the link may have expired, resume?". This is heuristic, it fires on
   files with a wrong duration, and it still interrupts the film. Rejected.
2. **Tell mpv to reload** over IPC (`loadfile` with the fresh URL and
   `start=<pos>`). mpv must first notice the failure, and it only reports `eof`.
   This also races the monitor's end-of-file handling. Rejected.
3. **Proxy.** mpv plays `http://127.0.0.1:<port>/<random>`; the proxy fetches
   the signed link and renews it when TorBox refuses it. mpv never sees the
   403. **Chosen.** The spike showed it works with the reconnect args unchanged.

Shape:

- `stream.Start(upstream, renew, opts...)` listens on `127.0.0.1:0`, behind a
  path from `crypto/rand.Text()` (≥128 bits). Any other path gets 404 and any
  method other than GET or HEAD gets 405. Neither ever reaches TorBox.
- It forwards `Range` and `If-Range`. It copies back the status and
  `Accept-Ranges`, `Content-Length`, `Content-Range`, `Content-Type`, `ETag` and
  `Last-Modified`.
- The transport sets `DisableCompression = true`. Go asks for gzip on a request
  without `Range`, then strips `Content-Length`, which leaves mpv unable to seek.
  A test pins this; the obvious version passed with compression on, because Go
  never asks for gzip when a `Range` is set.
- `ResponseHeaderTimeout` is 30s and there is no overall client timeout, since a
  stream runs for hours. `Close` ends in-flight streams.
- **Renewal.** A 401, 403, 404 or 410 from upstream calls `renew` once, then the
  request is retried once.
  - A generation counter under one mutex lets requests that expire together
    share one renewal.
  - Renewal is capped at 5 per playback and spaced at least 10s apart, so a link
    that is refused as soon as it is issued can't loop.
  - Each renewal runs on `context.WithoutCancel` with a 15s timeout: mpv
    dropping the triggering connection must not abort the renewal other requests
    are waiting on.
- **Renewal failure** passes the refusal status through, so mpv behaves as it
  does today, and sets `RenewFailed()`. The TUI and CLI turn that into
  `stream.ErrRenewFailed`. The TUI shows "Stream link expired and couldn't be
  renewed" instead of "Playback finished".
- **Logging:** only statuses and renewal counts, never links. `*url.Error`
  embeds the URL, so it is unwrapped (`withoutURL`). Renewer errors also go
  through `logging.Redact`, because the TorBox API key is a `requestdl` query
  parameter. G704 is suppressed on the one `client.Do`, with the reason.
- **`renew`** is the same closure that fetched the first link: `linkFetcher` in
  the TUI and `fetchLink` in the CLI. This also removes the duplicated category
  switch from `streamItemCmd` and `streamFileCmd`.
- **Config:** `player.stream_proxy`, default `true`. A config written before the
  key existed keeps it on, because `loadRaw` decodes onto `DefaultConfig()`.
  Setting it to `false` hands mpv the link directly, as before.

### 2. Failures sorted by what the user can do (`internal/tui/fetcherr.go`)

`classifyFetchErr` puts each error into one of three kinds:

- **auth rejected:** `torbox.IsUnauthorized` or `trakt.IsUnauthorized`
- **offline:** any `net.Error` (`*url.Error` is one), `context.DeadlineExceeded`,
  or a 5xx `APIError` from either service
- **other:** everything else. A 403 `PLAN_LIMIT`, for example, needs neither a
  new key nor waiting for the network.

**TorBox footer (FR-3.3.2):**

| Kind | Cached tab | No cache |
| --- | --- | --- |
| auth rejected | "TorBox rejected your API key — run: tt-wrapper auth torbox <key>"; cache stays on screen; the stale bit is **not** set | same |
| offline | unchanged: "Offline — showing data from …" | "Offline — couldn't load <tab>" (no URL) |
| other | unchanged | unchanged: "Failed to load <tab>: <err>" |

**Trakt header badge (FR-3.3.1).** The badge is the whole change. Pairing never
opens automatically, because an expired sign-in may not matter for what the user
is doing right now.

| State | Badge | Colour |
| --- | --- | --- |
| no token | `Trakt: [Not Paired]` | peach (unchanged) |
| last catalog fetch rejected | `Trakt: [Sign-in expired — press A]` | red |
| last catalog fetch offline | `Trakt: [Offline]` | yellow |
| last catalog fetch failed otherwise | `Trakt: [Unavailable]` | yellow |
| otherwise | `Trakt: [Connected ✓]` | green (unchanged) |

A later successful fetch or a new pairing (`TokenPollSuccessMsg`) clears the
warning. `pkg/trakt` now logs `trakt token refresh failed` at Warn on both
refresh paths, redacted.

## Consequences

- Every TorBox stream now goes through a loopback socket. That costs one extra
  local copy per byte. mpv's `launching player` debug log shows `127.0.0.1:<port>`
  as the host rather than the CDN.
- Another local user could reach the port but not the path, which comes from
  `crypto/rand`. The proxy only serves the one file.
- If the proxy can't listen, playback fails with that error rather than silently
  bypassing the proxy. On loopback this should never happen, and
  `stream_proxy = false` is the escape hatch.
- The TUI golden files are unchanged: the Connected badge and the footers that
  the goldens capture didn't change.

## After release

- **Real stream, 2026-09-28**, a 17.5 GB file: TorBox's CDN answers HTTP/1.1 206 to range requests. A 4.5-minute pause resumed on the same upstream connection, with no reconnect and no renewal. The only freeze seen was the maintainer's mpv window on Hyprland: video stuck while audio played. The most likely cause is the NVIDIA dGPU waking from runtime power management (`vo=gpu-next`, Vulkan, `hwdec=nvdec`), not the proxy.

## Open questions

- **TorBox's real expiry status is unverified.** 401/403/404/410 covers the
  usual CDN answers. The manual check in the plan (a stream left for more than
  3 hours, or paused) will show which one TorBox sends. If it's something else,
  such as a 400 or a redirect to an error page, it needs a one-line change to
  `linkRefused`.
- If Trakt starts rejecting a refresh for reasons other than revocation, the
  badge will say "Sign-in expired" for those too. That is acceptable, because
  re-pairing fixes both.

# ADR-0002: Resume positions are percentages, and resuming is offered, not assumed

- **Status:** Accepted
- **Date:** 2026-09-12
- **Affects:** `pkg/player` (`MediaStream`, `mpv.go`), `internal/tui`
  (`app.go`, new `modal_resume.go`, `types.go`)
- **Implements:** FR-3.4 (SPEC §3.4)
- **Related:** ROADMAP §3.7 and §3.8, the two v0.2.1 bugs this decision
  sits on top of; a local prototype, which settled the percent-seek
  question

## Context

Playing an item that Trakt already has a partial position for restarted it from
the beginning. Investigating turned up three separate gaps, not one.

**The unit is wrong.** `trakt.PlaybackItem.Progress` is a percentage. It flows
through `matcher.ProgressPercent` (`matcher.go:187`) into `item.TraktProgress`,
into `StreamURLResolvedMsg.ResumeSecs` (`app.go:893`), into
`player.MediaStream.ResumeSecs`, and is emitted as
`fmt.Sprintf("--start=%d", int(...))` (`mpv.go:108`) — seconds. A film stopped
at 41% relaunched at 00:00:41, which is indistinguishable from starting over.
The field name is the whole bug: a percentage was assigned to something called
`Secs` and nothing complained.

**Folder episodes never resumed at all.** `streamFileCmd` (`app.go:897`) never
set a resume position, despite the matcher maintaining a populated
`playbackByEpisode` index. Films and standalone files resumed (wrongly);
episodes did not resume at any value.

**The catalog went stale after playback.** `launchPlayerCmd`'s callback returns
`StatusMsg{Text: "Playback finished"}` and the `StatusMsg` handler
(`app.go:166`) returns no command, so nothing re-fetched. Only `Init()`, the
auth-success path, and the `r` key ever call `fetchTraktCatalogCmd`. The
progress was correct on Trakt and stale in `m.matcher` until the next `r` or a
restart.

There is **no race** in that third gap, which was the first hypothesis and is
worth recording as ruled out: `Monitor.Stop()` calls `m.wg.Wait()`
(`monitor.go:164`), `handleStop` sends the stop scrobble synchronously, and
`mpv.go` calls `c.Stop()` before closing `session.Done`, which is what
`playerExec.Run` waits on. Trakt has always been told before the TUI regains
control. Nothing needs a delay, a retry, or a completion signal.

Separately: resuming was silent. Even once correct, seeking straight to 41%
without asking is wrong when the user wanted to rewatch.

## Decision

**Resume positions are percentages, end to end.** `MediaStream.ResumeSecs`
becomes `ResumeAtPercent`, emitted as `--start=<pct>%`. Trakt stores no seconds
value, so the alternative would require the file's duration, which is available
only from mpv IPC *during* playback (`Monitor.duration`) — not at launch, which
is when the decision is made. Naming the field for its unit is what stops this
class of bug recurring.

**Resuming is offered, not assumed.** A fifth modal, `ModalResume`, shown when
the matcher already reports `StatusInProgress`:

```
┌──────────────────────────────────────────────┐
│ Resume Playback                              │
│                                              │
│   Blade Runner 2049                          │
│   You stopped 41% in, 3 days ago.            │
│                                              │
│ [r] Resume at 41%   [s] Start over   [esc]   │
└──────────────────────────────────────────────┘
```

- `[r]` resume, `[s]` start over, `[esc]` cancel, `Enter` defaults to resume.
- Content is the percentage plus a relative `PlaybackItem.PausedAt`, a field
  that exists and was previously unused. No timestamp: there is no duration.
- **No position means no prompt.** Playback starts immediately, no keypress.
- Shown **before** resolving the stream link, so it is instant and cancelling
  costs no TorBox round-trip.

**The gate is `StatusInProgress`, reused verbatim** — `progress > 0 &&
progress < scrobbleThreshold`, the same predicate that drives the list badge.
The prompt and the badge therefore cannot disagree: if the list says in
progress, play offers a resume.

**"Start over" does not call `RemovePlayback`.** It means "play from zero now",
not "forget this". Clearing first would destroy a real 41% position if the user
then quit two minutes in. The scrobbler overwrites naturally.

**After playback, a dedicated `PlaybackFinishedMsg`** replaces the generic
`StatusMsg`, handled in `Update` by batching `fetchTraktCatalogCmd` alone. The
TorBox listing is not re-fetched — watching changes nothing about download
state, size or seeds — and the refresh is silent: no loading state, no
"Refreshing library…" flash. The existing "Playback finished" text stays.

**No config field.** `player.resume_prompt` would exist only to disable a
feature that was just requested, and Phase 7 explicitly wants every config
field read by some code path. Add one if the keypress proves annoying.

**TUI only.** `tt-wrapper stream` is out of scope.

## Consequences

**Two v0.2.1 bug fixes land first**, tracked as ROADMAP §3.7 (the unit
and the missing episode wiring) and §3.8 (the stale catalog). §3.8 is a
prerequisite for the prompt being trustworthy: without it you could finish a
film at 95%, relaunch, and be offered a resume from before you watched it.

**`r` remains the manual refresh** and is unchanged. It is also the existing
workaround for §3.8, which is why the bug reads as "have to restart the app"
rather than "progress is lost".

**Verified 2026-09-12: percent-seeking is no worse than seconds.** Probed with
a local prototype against two real TorBox streams, launching mpv
twice per point — `--start=<pct>%` and `--start=<seconds>` from duration as a
control — and reading the landing position back over `player.DialIPC`. Percent
and seconds drifted *identically* at every point: 0.0s/0.0s on a 22.4 GiB 4K
file at 2/5/41/75/95/99%, and 0.0s/0.0s, 18.4s/18.4s, 54.3s/54.3s on a 1.4 GiB
episode. The drift is inherent to seeking these streams, not to the unit, so
the IPC-seek fallback contemplated above is **not needed** and percent stands.

**Known imprecision: a resume can land up to ~18s late inside the operating
band.** Drift tracks the container's index quality, not file size — the 22 GiB
4K file was exact everywhere while the 1.4 GiB episode was not. The worst
observed case, 54.3s at 95%, falls above the default 90% scrobble threshold, so
the matcher marks it watched and never offers a resume. Within the band that
does get offered, the worst observed is 18.4s at 5%: a modal saying "Resume at
5%" may actually land at 6.28%. Accepted — the feature exists so you do not
lose your place, and ~18s does not lose it — but it is a measured limitation,
not an assumption. Do not let a future change present the resume position as
frame-accurate.

**Deliberately not solved: which episode to play next.** ROADMAP §4.2's
"Multi-file resume" is adjacent and distinct — it is about picking the next
episode in a folder, where this ADR is about the position within whatever was
chosen. Nothing here answers it.

**Follow-up recorded, not dropped:** a CLI `--resume` flag. `cli/stream.go`
builds a scrobbler and reports progress to Trakt but never fetches the playback
catalog and never builds a `Matcher`, so it feeds Trakt data it cannot consume.
Wiring that catalog in is the real cost, which is why it is not in this change.

## Alternatives considered

- **Convert percent to seconds at launch.** Rejected: needs a duration nothing
  has before playback starts.
- **Keep resuming silently, now that it is correct.** Rejected by the user, who
  asked for the choice; rewatching is a real case.
- **A `player.resume_prompt` config field.** Rejected as speculative dead
  config, the exact pattern Phase 7 exists to prevent.
- **Clear the Trakt playback entry on "start over".** Rejected: loses a good
  position in exchange for a worse one if the user quits early.
- **Prompt after resolving the stream link.** Rejected: a modal that appears a
  second late feels like a different application, and cancelling would have
  already burned an API call.
- **Delay or retry the post-playback refresh.** Rejected: no race exists, as
  traced above.

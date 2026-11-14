# ADR-0004: The playback monitor is driven by mpv property events, and scrobbles are coalesced

- **Status:** Accepted and shipped in **v0.4.0** on 2026-09-27 (PR #9), with
  follow-ups in v0.4.1 (PR #15).
- **Date:** 2026-09-26
- **Amends:** DESIGN §3 — drops the 15s watchdog (§3.2 item 4) and
  qualifies "reports both" (§3.3) with coalescing and spacing
- **Affects:** `pkg/player/ipc.go`, `pkg/player/monitor.go`, a new
  `pkg/player/scrobblestate.go`
- **Implements:** FR-3.2.1 – FR-3.2.6 (SPEC §3.2)
- **Related:** a local prototype, which measured the event rates
  below against mpv 0.41

## Context

`Monitor.poll` asks mpv for `time-pos`, `percent-pos` and `pause` once a second,
plus `duration` until it has one. That is three round-trips a second while
nothing is happening, and a pause and resume inside one second is invisible, so
Trakt never hears about it (DESIGN §3.3). `IPCClient` already reads mpv's
unsolicited events into `OnEvent` hooks, and nothing subscribes.

**Measured 2026-09-26** with mpv 0.41 on a lavfi test source, observing the four
properties:

| Window | `time-pos` | `percent-pos` | `pause` | `duration` |
| --- | --- | --- | --- | --- |
| 4s playing | 96 | 96 | 0 | 79 |
| 3s paused | 0 | 0 | 0 | 0 |

`duration` only moves because a lavfi source grows; a real file reports it once.
Pause toggles 0.3s apart all arrived as separate edges, and each seek produced
exactly one `playback-restart`.

`observe_property` answers immediately with the current value, before any
change, so a hook registered after the observe call misses the initial state.
mpv sends a `property-change` with no `data` when a property becomes
unavailable: on quit, `time-pos`, `percent-pos` and `duration` all did, just
before `end-file`.

Two facts constrain what reaches Trakt. Trakt allows about one authenticated
write a second and answers faster bursts with 429. And the Stop scrobble is the
one that marks an item watched; losing it to a 429 is the worst outcome
available.

Two existing behaviours were wrong and are fixed here because events make it
cheap. A Start was sent whenever `time-pos > 0`, including when mpv opened
paused (`pause=yes` in `mpv.conf`), telling Trakt the user was watching when
they were not. And a seek never reached Trakt, so its "watching now" progress
drifted from the player's until the next pause.

## Decision

### Events feed state; one loop decides

The monitor observes `time-pos`, `percent-pos`, `pause` and `duration`, and
listens for the `playback-restart` event. The hooks run on the IPC reader
goroutine and do exactly two things: write the new value into the monitor's
snapshot under `mu`, and poke a one-slot `wake` channel without blocking. They
never make a network call and never block, so a slow Trakt response cannot
stall the socket.

`run` is the only goroutine that talks to Trakt. On each wake it compares the
snapshot against what Trakt was last told and sends at most one call.

Hooks are registered **before** the observe commands, so the immediate replies
carrying the initial values are not lost. An event with no `data` leaves the
previous value in place, so the Stop scrobble still carries the last real
position.

### Scrobble rules

What Trakt was last told is one of *nothing*, *playing* or *paused*.

| Rule | When | Sends |
| --- | --- | --- |
| Start | told nothing, not paused, not seeking, `time-pos > 0`, `percent-pos > 0` | `Start(percent)` |
| Pause | told playing, paused | `Pause(percent)` |
| Resume | told paused, playing, not seeking | `Start(percent)` |
| Seek | told playing, playing, not seeking, `playback-restart` since the last Start | `Start(percent)` |
| Stop | stop signal or context done, and a Start was ever sent | `Stop(final percent)` |

- **Start waits for real playback.** Opened paused and never unpaused means no
  Start and therefore no Stop.
- **Comparison happens at send time**, not per event. That is the coalescing: a
  pause, resume, pause burst ends as one Pause. A single pause then resume
  still sends both, which is DESIGN §3.3's requirement, provided the pause lands
  at least 1s after the previous call returned. Inside that window the pair
  collapses to nothing, like any other burst; Trakt was told playing and the
  player is playing, so the drift is under a second.
- **Seek while paused sends nothing**; the next Resume or Stop carries the new
  position. `playback-restart` also fires on file load and after the
  `--start=<pct>%` resume jump, so it is ignored until a Start has been sent.
- **Spacing.** A call may not start until 1s after the previous one returned.
  A call that is due early arms a timer for the moment it is allowed; wakes
  in the meantime only update the snapshot.
- **Stop honours the spacing**, waiting at most 1s, so the call that marks an
  item watched is never the one that draws a 429. It supersedes any call still
  waiting, cancels any Start or Pause still in flight, and keeps today's 10s
  `WithoutCancel` timeout. Start and Pause are bounded at 10s too.
- **A failed call still updates what Trakt was told.** The error is logged as
  today. Retrying would loop once a second on a title that never resolves;
  retry policy is Phase 5's.

These rules live in a pure `scrobbleState` type (`scrobblestate.go`) that takes
a snapshot and a time and returns the next action and how long to wait. It
holds no socket, goroutine or clock of its own, so every rule is a table test
against a fake clock.

### Fallback

If any `observe_property` command fails on attach, the monitor logs once and
runs the 1s poll loop for the rest of the session. The poll writes the same
snapshot and pokes the same `wake` channel, so the rules above, including
Start-only-when-playing and spacing, apply unchanged. Seek re-scrobbling is
event-mode only.

There is **no watchdog** in event mode. Over a local Unix socket events are not
lost, and a 15s re-read would be the only IPC traffic left while paused, which
is the traffic this phase exists to remove.

### Progress callback

`SetProgressCallback` keeps its signature and still receives the latest
snapshot, now on every wake (about 50 a second while playing) rather than once a
second. It has no production caller; its doc comment says it must be cheap.

## Consequences

- IPC traffic from the monitor while paused drops from 3–4 requests a second to
  zero. Under test this is an assertion, not a manual observation.
- A pause shorter than a second reaches Trakt, unless it starts within a
  second of the previous call. A burst of toggles reaches it as
  its final state, at no more than one call a second.
- Quitting within a second of a pause or resume delays the TUI regaining
  control by up to 1s, the price of never losing the Stop.
- A user whose `mpv.conf` opens paused no longer appears on Trakt as watching
  until they press play. This is a behaviour change, so the release is v0.4.0.
- `monitor.go` holds lifecycle and sources; `scrobblestate.go` holds rules. The
  decision logic that was inline in `poll` moves out.

## Alternatives considered

- **Every event on a channel.** About 50 events a second while playing; if
  `run` is inside a 10s Trakt call, the buffer must hold ~500 or drop some, and
  a dropped `pause` is the bug being fixed.
- **A separate scrobble worker with its own queue.** Another goroutine, and
  `Stop` would have to drain the queue and still send the final Stop in order.
  Coalescing in one loop is simpler.
- **Send every edge.** Most literal reading of DESIGN §3.3, but a toggle burst
  draws 429s and those calls are lost.
- **Debounce pauses under ~2s.** Fewest calls, and reintroduces the invisible
  short pause.
- **15s watchdog in event mode** (DESIGN §3.2 as written). Guards against a
  stuck player, costs a second code path, and keeps IPC traffic while paused.
- **Watchdog as the only fallback.** A pause under 15s would go unseen on any
  mpv that rejects `observe_property`.

## Amendments after review (2026-09-27)

Built on `feat/event-monitor` (`e991fa2`, `b4cf2a6`, `f66524a`, `9ff3dfd`). The
whole-branch review changed three things, all reflected above:

- **The first Start also waits for `percent-pos > 0`.** On a `--start=N%` resume
  the monitor attaches before the file loads, and `time-pos` arrives as its own
  event ahead of `percent-pos`, so `run` could send `Start(0)`. Gating on
  `duration > 0` would not help: `duration` is observed last. This matches the
  old poll loop, which never started while `percent-pos` was unavailable.
- **Stop cancels an in-flight Start or Pause**, and both are bounded at 10s. A
  title search plus a scrobble can take up to 60s on Trakt's client timeout, and
  `Monitor.Stop` blocked the TUI on it. `Stop` cancels before signalling, so no
  call begins once Stop has started.
- **A pause and resume inside the spacing window collapse to nothing.** This is
  the send-time comparison applied to a pair; FR-3.2.5 was reworded to say so.

Measured on mpv 0.41 (lavfi, through the real `Monitor`,
a local prototype): 4 `observe_property` commands on attach, then
0 IPC requests for the whole session. Opened paused and quit: no Trakt call.

### Amendment after the real-stream check (2026-09-27)

The maintainer's TorBox resume (`--start=3.3%`) sent two Starts at 3.3%. On
mpv 0.41 a resume reports `time-pos`/`percent-pos` while still seeking to the
start point (percent reads 100 on a lavfi source), then `playback-restart`,
then `seeking=false`. The first Start went mid-seek and the restart re-sent it.

Fixed in `67d8798`: **no Start is sent while mpv reports `seeking`**; Pause is
not held. `seeking` is observed first, because mpv delivers one batch of
changes in observation order and must report the seek before the mid-seek
positions; `playback-restart` also always arrives before `seeking=false`, since
mpv's client queue flushes events before property changes. The first restart
therefore lands before the first Start and is absorbed. Waiting for the first
`playback-restart` instead was rejected: on a local file mpv is already playing
when the monitor attaches, so that event is never seen. The poll fallback
leaves `seeking` false, as before. Measured on a real 120s file through the
harness: resume at 30s and at 50% each send one Start (25.00%, 50.03%); plain
play is unchanged; 5 `observe_property` commands on attach, then 0 requests.

### Deferred

- **The monitor can outlive mpv.** In `mpv.go`, the controller is stored before
  `Monitor.Start`, and mpv can exit between dialling and `Start`. In either
  window, `cmd.Wait`'s `Stop` misses the run goroutine, which then polls a
  closed client until the app context ends. It is pre-existing and the window
  is microseconds wide. The fix is for `run` to also exit when the IPC client
  closes. *Fixed in v0.4.1 (#10): `run` ends on `IPCClient.Done()`, sending
  the final Stop if a Start went out.*
- The `pkg/player` suite now takes ~23s because monitor tests wait out the real
  1s spacing; `t.Parallel` would recover most of it. *Fixed in v0.4.1 (#14):
  ~26s → ~4s.*

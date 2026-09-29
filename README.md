# torbox-trakt-wrapper (`tt-wrapper`)

[![Go Reference](https://pkg.go.dev/badge/github.com/manojpannala/torbox-trakt-wrapper.svg)](https://pkg.go.dev/github.com/manojpannala/torbox-trakt-wrapper)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![CI](https://github.com/manojpannala/torbox-trakt-wrapper/actions/workflows/ci.yml/badge.svg)](https://github.com/manojpannala/torbox-trakt-wrapper/actions/workflows/ci.yml)

A high-performance terminal client and TUI for browsing, streaming, and managing your **TorBox** cloud library with real-time **Trakt.tv** synchronization.

---

## ✨ Features

- **⚡ Direct CDN Media Streaming**: Streams directly into `mpv` via TorBox fast CDN links without WebDAV bottlenecks or FUSE mount latency.
- **🔄 Seamless Trakt.tv Synchronization**:
  - Headless OAuth Device Code Pairing (auto-copies pairing codes to system clipboard).
  - Background playback tracking & scrobbling via isolated MPV IPC Unix sockets.
  - Automatic watch status markers (`✓` watched, `◐` resume progress %) across movies, TV seasons, and multi-file torrents.
- **🎨 Catppuccin Mocha TUI**:
  - Interactive multi-tab browser (`[1] Torrents`, `[2] Usenet`, `[3] Web-DL`).
  - Multi-file torrent & TV series folder tree explorer.
  - Instant live fuzzy search/filter (`/`).
  - Modals for magnet adding (with clipboard auto-paste), deletion confirmation, resume-or-restart, Trakt pairing, and help cheat sheet.
- **🧹 Intelligent Media Parser**:
  - Cleans release scene tags (`2160p`, `Remux`, `HEVC`, `DDP5.1`, `TrueHD`, `HDR`, `AV1`).
  - Right-to-left reverse year extraction, so titles that themselves contain or consist of a four-digit number still resolve to the correct release year.
  - Robust TV episode and season recognition (`S01E05`, `S01E01-E04`, `1x05`, anime flat notation).
- **🔎 Optional Release Search**: Search your own [Prowlarr](https://prowlarr.com) for a title's releases, see which are already cached on TorBox (`●` cached, `○` not cached, `■` in your library), and add one in a keypress.
- **💻 Dual Interface**: Interactive Bubble Tea TUI or scriptable, headless CLI subcommands (`auth`, `list`, `add`, `stream`, `search`, `config`).
- **🛡️ Privacy & Security**:
  - Zero plain-text token leaks in process arguments.
  - Isolated per-session IPC sockets with `0700` filesystem permissions.
  - Automatic token expiration handling and silent proactive token refresh.

---

## 📦 Installation

### Go Install

```bash
go install github.com/manojpannala/torbox-trakt-wrapper/cmd/tt-wrapper@latest
```

### Build from Source

```bash
git clone https://github.com/manojpannala/torbox-trakt-wrapper.git
cd torbox-trakt-wrapper
make build
# Binary is generated at bin/tt-wrapper
```

---

## 🚀 Quickstart

1. **Launch the TUI**:
   ```bash
   tt-wrapper
   ```
2. **Set your TorBox API Key**:
   ```bash
   tt-wrapper auth torbox <your_api_key>
   ```
3. **Pair your Trakt Account**:
   Press <kbd>A</kbd> in the TUI or run:
   ```bash
   tt-wrapper auth trakt
   ```

---

## ⌨️ TUI Keybindings

| Key | Action |
| --- | --- |
| <kbd>Enter</kbd> / <kbd>Space</kbd> | Stream selected media / open file in MPV |
| <kbd>Tab</kbd> / <kbd>1</kbd>, <kbd>2</kbd>, <kbd>3</kbd> | Switch category tabs (`Torrents` / `Usenet` / `Web-DL`) |
| <kbd>f</kbd> / <kbd>o</kbd> | Open multi-file torrent / folder file tree explorer |
| <kbd>/</kbd> | Focus instant fuzzy search / filter bar |
| <kbd>Esc</kbd> | Clear search / close active modal / back to library |
| <kbd>a</kbd> | Add new download (Magnet link / URL) |
| <kbd>s</kbd> | Search Prowlarr for releases (see [Search](#-search-optional)) |
| <kbd>S</kbd> | Search for other releases of the selected title |
| <kbd>d</kbd> / <kbd>x</kbd> | Delete selected download confirmation |
| <kbd>p</kbd> | Pause / resume the selected download |
| <kbd>r</kbd> | Refresh library list and Trakt watch history |
| <kbd>A</kbd> | Open Trakt.tv OAuth device code pairing modal |
| <kbd>?</kbd> | Toggle keyboard shortcuts help overlay |
| <kbd>q</kbd> / <kbd>Ctrl+C</kbd> | Quit application |

Playing something Trakt has a partial position for opens a resume prompt first:
<kbd>r</kbd> resumes, <kbd>s</kbd> starts over, <kbd>Esc</kbd> cancels. Inside
that prompt <kbd>r</kbd> means resume, not refresh.

---

## 🔎 Search (optional)

Search is off until you point it at a [Prowlarr](https://prowlarr.com) you run
yourself. The wrapper ships no indexers of its own: it asks Prowlarr, and only
the torrent indexers you have enabled there.

```toml
[search]
prowlarr_url = "http://127.0.0.1:9696"
prowlarr_api_key = "your_prowlarr_api_key" # Prowlarr → Settings → General → API Key
```

`prowlarr_url` may use `http` only for a Prowlarr on this machine
(`localhost`, `127.0.0.1`, `::1`); anything else must use `https`. The key
can also come from `PROWLARR_API_KEY`.

In the TUI, press <kbd>s</kbd> and type a title. Trakt finds the film or show;
pick it, and for a show type `S02`, `S02E05`, or leave it blank for the whole
show. You can also type an IMDb ID (`tt0000000 S01E02`) to skip that step, or
press <kbd>Ctrl+R</kbd> to send the words to Prowlarr exactly as typed.
<kbd>S</kbd> on a library row searches for other releases of what it matched
on Trakt. Title search needs a Trakt `client_id` under `[trakt]`; without one,
search by IMDb ID or with <kbd>Ctrl+R</kbd>.

Each release shows a badge: `●` cached on TorBox (it streams at once), `○` not
cached, `■` already in your library, `…` still getting its magnet, `?` unknown.
Cached checks are batched and paced to stay well under TorBox's rate limit, so
badges can take a moment to fill in.

| Key | Action |
| --- | --- |
| <kbd>Enter</kbd> | Add the release to TorBox (or select it, if it's already in your library) |
| <kbd>/</kbd> | Filter the releases |
| <kbd>O</kbd> | Sort by cached, size, seeders or resolution |
| <kbd>s</kbd> | New search |
| <kbd>Esc</kbd> | Back |

---

## 🛠️ CLI Subcommands

```bash
# Authenticate
tt-wrapper auth torbox <api_key>
tt-wrapper auth trakt

# List downloads (with Trakt watched status badges)
tt-wrapper list torrents
tt-wrapper list usenet
tt-wrapper list webdl --json

# Queue a download
tt-wrapper add "magnet:?xt=urn:btih:..."
tt-wrapper add "https://example.com/file.nzb"
tt-wrapper add "https://example.com/video.mp4"

# Search Prowlarr (needs [search] in the config)
tt-wrapper search "Some Film"            # titles and their IMDb IDs
tt-wrapper search tt0000000              # releases, cached first
tt-wrapper search "tt0000000 S01E02" --sort size --json
tt-wrapper search --raw "words as typed"

# Direct stream matching query
tt-wrapper stream "Interstellar"
tt-wrapper stream 102

# Inspect or initialize configuration
tt-wrapper config
tt-wrapper config path
tt-wrapper config init

# Delete cached listings and watch history
tt-wrapper cache clear
```

### Global flags

| Flag | Effect |
| --- | --- |
| <kbd>-c</kbd>, `--config <path>` | Use a specific config file |
| <kbd>-v</kbd>, `--verbose` | Write a debug log (see below) |

### Debug logging

`--verbose` works on every command, including the TUI, and appends to:

```
$XDG_STATE_HOME/torbox-trakt-wrapper/tt-wrapper.log   # ~/.local/state/... by default
```

The file is created at `0600` and only when `--verbose` is passed — a normal run
writes nothing. It records API requests (method, path, status, duration), the
flags `mpv` was launched with, and scrobble decisions.

API keys and tokens are never written. Request bodies and headers are never
logged, credential query parameters are blanked to `REDACTED` (TorBox's
download-link endpoints carry the key as `token=`), and the signed stream URL is
reduced to its host. Attach it to a bug report as-is.

### Cache

Your TorBox listings and Trakt watch history are kept in
`$XDG_CACHE_HOME/torbox-trakt-wrapper/` (`~/.cache/...` by default), so the
library appears the moment the app opens and then refreshes in the background.

- Files are `0600` and never contain API keys, tokens or stream links.
- `torbox.cache_ttl_minutes` sets how old a tab you switch to may be before it
  is refreshed. The tab you open into is always refreshed.
- Pairing a different TorBox or Trakt account clears that account's cached data.
- `tt-wrapper cache clear` deletes everything.

---

## ⚙️ Configuration

Configuration is located at `$XDG_CONFIG_HOME/torbox-trakt-wrapper/config.toml` (defaults to `~/.config/torbox-trakt-wrapper/config.toml`):

```toml
[torbox]
api_key = "your_torbox_api_key"
default_category = "torrents" # torrents | usenet | webdl
cache_ttl_minutes = 15

[trakt]
client_id = "" # optional custom Trakt API client ID
client_secret = "" # optional custom Trakt API client secret
access_token = ""
refresh_token = ""
token_created_at = 0
token_expires_in = 0

[player]
command = "mpv"
args = [
    "--force-seekable=yes",
    "--resume-playback=no",
    "--save-position-on-quit=no",
    "--stream-lavf-o=reconnect=1,reconnect_streamed=1,reconnect_delay_max=30"
]
enable_ipc = true
scrobble_threshold_percent = 90
keep_open = "" # "" leaves mpv.conf alone | "yes" | "no" | "always"
stream_proxy = true # renew an expired TorBox link mid-stream; false hands mpv the link directly

[ui]
theme = "catppuccin-mocha"
show_unwatched_badge = false
compact_mode = false

[search] # optional; see Search above
prowlarr_url = ""
prowlarr_api_key = ""
```

### Environment Variables

All settings can be overridden using environment variables:
- `TORBOX_API_KEY`
- `TRAKT_CLIENT_ID`
- `TRAKT_CLIENT_SECRET`
- `TRAKT_ACCESS_TOKEN`
- `TRAKT_REFRESH_TOKEN`
- `PROWLARR_API_KEY`

---

## 🤝 MPV Integration & Custom Dotfiles

`tt-wrapper` seamlessly cooperates with your custom `mpv` dotfiles (`mpv.conf`, custom Lua scripts, shaders, Vulkan/GPU pipelines, tone-mapping). It launches MPV with an isolated Unix IPC socket to monitor playback progress without altering your player keybindings or script states.

### `keep-open` and deferred scrobbles

If your `mpv.conf` sets `keep-open=always`, mpv does not exit at the end of a
file. `tt-wrapper` waits for the player to exit before sending the final Trakt
scrobble, so the watch is not recorded until you close mpv yourself. Set
`keep_open = "no"` under `[player]` to append `--keep-open=no` for wrapper
launches only, leaving your `mpv.conf` untouched for everything else.

---

## 📄 License

Distributed under the [MIT License](LICENSE).

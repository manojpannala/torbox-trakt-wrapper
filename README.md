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
  - Modals for magnet adding (with clipboard auto-paste), deletion confirmation, Trakt pairing, and help cheat sheet.
- **🧹 Intelligent Media Parser**:
  - Cleans release scene tags (`2160p`, `Remux`, `HEVC`, `DDP5.1`, `TrueHD`, `HDR`, `AV1`).
  - Right-to-left reverse year extraction to avoid title collisions (*2001: A Space Odyssey*, *Blade Runner 2049*, *1917*).
  - Robust TV episode and season recognition (`S01E05`, `S01E01-E04`, `1x05`, anime flat notation).
- **💻 Dual Interface**: Interactive Bubble Tea TUI or scriptable, headless CLI subcommands (`auth`, `list`, `add`, `stream`, `config`).
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
| <kbd>d</kbd> / <kbd>x</kbd> | Delete selected download confirmation |
| <kbd>r</kbd> | Refresh library list and Trakt watch history |
| <kbd>A</kbd> | Open Trakt.tv OAuth device code pairing modal |
| <kbd>?</kbd> | Toggle keyboard shortcuts help overlay |
| <kbd>q</kbd> / <kbd>Ctrl+C</kbd> | Quit application |

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

# Direct stream matching query
tt-wrapper stream "Interstellar"
tt-wrapper stream 102

# Inspect or initialize configuration
tt-wrapper config
tt-wrapper config path
tt-wrapper config init
```

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

[ui]
theme = "catppuccin-mocha"
show_unwatched_badge = false
compact_mode = false
```

### Environment Variables

All settings can be overridden using environment variables:
- `TORBOX_API_KEY`
- `TRAKT_CLIENT_ID`
- `TRAKT_CLIENT_SECRET`
- `TRAKT_ACCESS_TOKEN`
- `TRAKT_REFRESH_TOKEN`

---

## 🤝 MPV Integration & Custom Dotfiles

`tt-wrapper` seamlessly cooperates with your custom `mpv` dotfiles (`mpv.conf`, custom Lua scripts, shaders, Vulkan/GPU pipelines, tone-mapping). It launches MPV with an isolated Unix IPC socket to monitor playback progress without altering your player keybindings or script states.

---

## 📄 License

Distributed under the [MIT License](LICENSE).

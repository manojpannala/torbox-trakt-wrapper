# torbox-trakt-wrapper (`tt-wrapper`)

A terminal user interface (TUI) and CLI for browsing, streaming, and managing your **TorBox** cloud library with seamless **Trakt.tv** synchronization.

## Features

- **Direct Streaming**: Stream media directly to `mpv` via TorBox CDN links without FUSE mounts or WebDAV bottlenecks.
- **Trakt.tv Integration**: Terminal-based OAuth device authentication, automatic background token refresh, watched indicators, and playback scrobbling.
- **Library Management**: Browse Torrents, Usenet, and Web Downloads. Add magnet links, view progress, and delete items.
- **Smart Media Matcher**: Strips scene release tags and matches titles and episodes to Trakt catalog entries.
- **Dual Mode**: Interactive Bubble Tea TUI or scriptable CLI subcommands.

## Installation

### From Source

```bash
git clone https://github.com/manojpannala/torbox-trakt-wrapper.git
cd torbox-trakt-wrapper
make build
# Binary is available at bin/tt-wrapper
```

Or install directly to `$GOPATH/bin`:

```bash
go install ./cmd/tt-wrapper
```

## Configuration

Configuration is stored at `$XDG_CONFIG_HOME/torbox-trakt-wrapper/config.toml` (default `~/.config/torbox-trakt-wrapper/config.toml`):

```toml
[torbox]
api_key = "your_torbox_api_key"
default_category = "torrents" # torrents | usenet | webdl
cache_ttl_minutes = 15

[trakt]
client_id = "" # optional override
client_secret = "" # optional override
access_token = ""
refresh_token = ""
token_created_at = 0
token_expires_in = 0

[player]
command = "mpv"
args = [
    "--vfs-cache-max-size=5G",
    "--force-media-title=${TITLE}"
]
enable_ipc = true
scrobble_threshold_percent = 90

[ui]
theme = "catppuccin-mocha"
show_unwatched_badge = false
compact_mode = false
```

### Environment Variables

You can also configure credentials via environment variables:

- `TORBOX_API_KEY`
- `TRAKT_CLIENT_ID`
- `TRAKT_CLIENT_SECRET`
- `TRAKT_ACCESS_TOKEN`
- `TRAKT_REFRESH_TOKEN`

## CLI Usage

```bash
# Launch interactive TUI
tt-wrapper

# Authenticate TorBox or Trakt
tt-wrapper auth trakt
tt-wrapper auth torbox

# List items from cloud library
tt-wrapper list torrents
tt-wrapper list usenet --json

# Add download via magnet URL
tt-wrapper add "magnet:?xt=urn:btih:..."

# Stream media matching query
tt-wrapper stream "Interstellar"

# Show current configuration
tt-wrapper config
```

## License

[MIT](LICENSE)

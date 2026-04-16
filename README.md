# go-discord-music

A Discord music bot written in Go. Streams audio from YouTube into voice channels using `yt-dlp`, `ffmpeg`, and `dca` for Opus encoding.

## Features

- **Playlist support** — playlists load asynchronously; playback starts on the first track while the rest stream in the background
- **Local cache download** — tracks are downloaded to disk before encoding to avoid direct-pipe freezing issues with YouTube
- **Auto-reconnect** — if the bot drops from a voice channel (Discord server migration, Gateway blip), it rejoins and resumes automatically
- **Per-server state** — queue, volume, history, and search results are isolated per guild
- **Duration filter** — tracks over a configurable limit (default 10 minutes) are skipped
- **DAVE (E2EE) compatible** — includes a brief handshake delay before sending audio frames
- **Saved playlists** — queues can be saved to and loaded from disk by name

## Commands

### Playback

| Command | Description |
|---------|-------------|
| `!play <URL or query>` | Play a YouTube URL or search for a track. Playlists shuffle by default. |
| `!search <query>` | Show the top 20 results; pick one with `!p <number>`. |
| `!np` / `!playing` | Show the currently playing track. |
| `!skip` / `!next` | Skip to the next track. |
| `!previous` / `!prev` | Go back to the previous track. |
| `!pause` | Pause playback. |
| `!resume` | Unpause, or restart the queue after a disconnect. |
| `!stop` | Stop playback and clear the queue. |
| `!volume <1-500>` | Set volume (percentage, applied via FFmpeg filter). |

### Queue

| Command | Description |
|---------|-------------|
| `!queue` / `!q` | Show the queue (first 15 tracks). `!queue all` shows everything. |
| `!clear` | Clear the queue. |
| `!shuffle` | Shuffle the current queue in place. |
| `!move <from> <to>` | Move a track by index. |
| `!remove <index>` | Remove a track by index. |

### Playlists & Session

| Command | Description |
|---------|-------------|
| `!savequeue <name>` | Save the current queue under a name. |
| `!loadqueue <name>` | Load a saved queue. |
| `!savedplaylists` | List all saved playlists for this server. |
| `!join` | Join your current voice channel. |
| `!leave` | Leave and clear the queue. |

## Configuration

Copy `.env.sample` to `.env` and fill in the values:

```env
DISCORD_BOT_TOKEN="your_bot_token"
COMMAND_PREFIX="!"               # default: !
MAX_TRACK_DURATION=600           # seconds; set 0 to disable (default: 600 = 10 min)
PUID=1000
PGID=1000
```

## Running

```bash
docker-compose up -d --build
```

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/bwmarrin/discordgo` | Discord Gateway and voice WebSocket |
| `github.com/jonas747/dca` | Opus audio encoding |
| `github.com/joho/godotenv` | `.env` file loading |
| `yt-dlp` | YouTube audio extraction (system binary) |
| `ffmpeg` | Audio transcoding (system binary) |

## License

AGPL-3.0-or-later — see [LICENSE](LICENSE).

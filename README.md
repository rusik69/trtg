# TRTG - Torrent to Telegram

A Go application that downloads files from torrents (magnet links or .torrent files) and uploads them to Telegram using Local Bot API Server (supports up to 2GB files).

## Features

- Reads torrent URLs (magnet links or .torrent file paths) from a configuration file
- Downloads torrents sequentially using anacrolix/torrent library
- **Supports up to 2GB files** via Local Bot API Server
- Uploads files to Telegram as documents
- Tracks downloaded torrents in PostgreSQL database to avoid duplicates
- Supports dry-run mode to preview which torrents would be downloaded
- Automatic cleanup of downloaded files after successful upload
- Docker support with easy deployment

## Quick Start with Docker

1. Create `prod.env` with your credentials:
```bash
TELEGRAM_API_ID=your_api_id
TELEGRAM_API_HASH=your_api_hash
TELEGRAM_BOT_TOKEN=your_bot_token
TELEGRAM_CHAT_ID=your_chat_id
```

2. Add torrents to `torrents.txt` (one per line, magnet links or .torrent file paths)

3. Run locally:
```bash
cp prod.env .env
docker-compose up
```

## Deployment to Production

### First-time setup:
```bash
# Setup remote server (install Docker)
make deploy-setup

# Create prod.env with your credentials
cp prod.env.example prod.env
# Edit prod.env with your values

# Deploy
make deploy
```

### Deployment commands:
```bash
make deploy          # Full deployment (copy + start)
make deploy-stop     # Stop services
make deploy-logs     # View logs
make deploy-status   # Check status
make deploy-run-once # Run once manually
make deploy-clean    # Remove from server
```

## Prerequisites

### For Docker
- Docker and Docker Compose

### For Local Development
- Go 1.23 or later
- A Telegram bot token (from [@BotFather](https://t.me/botfather))
- Your Telegram chat ID
- [Local Bot API Server](https://github.com/tdlib/telegram-bot-api)

## Configuration

### Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `TELEGRAM_BOT_TOKEN` | Bot token from @BotFather | Yes |
| `TELEGRAM_CHAT_ID` | Chat ID to send files to | Yes |
| `TELEGRAM_API_URL` | Local Bot API Server URL | Yes |
| `TELEGRAM_API_ID` | API ID from my.telegram.org | For Local API |
| `TELEGRAM_API_HASH` | API Hash from my.telegram.org | For Local API |
| `TORRENTS_FILE` | Path to torrents file | No (default: torrents.txt) |
| `DATABASE_URL` | PostgreSQL connection URL | No (default: postgres://trtg:trtg@127.0.0.1:5432/trtg?sslmode=disable) |
| `DOWNLOAD_DIR` | Download directory | No (default: downloads) |

### Files

| File | Description | In Git |
|------|-------------|--------|
| `torrents.txt` | Torrent URLs (magnet links or .torrent paths) | Yes |
| `prod.env` | Production credentials | No |
| `.env` | Local credentials | No |

### Getting Telegram Credentials

1. **Bot Token**: Create a bot via [@BotFather](https://t.me/botfather)
2. **Chat ID**: Message [@userinfobot](https://t.me/userinfobot) to get your user ID
3. **API ID/Hash**: Get from [my.telegram.org](https://my.telegram.org)

**Important:** Start a conversation with your bot before running.

### Torrents File

```
# Lines starting with # are comments
magnet:?xt=urn:btih:...
/path/to/file.torrent
```

## Command Line Options

```
-torrents string    Path to torrents file
-db string          PostgreSQL connection URL
-download-dir       Download directory
-dry-run            Preview without downloading
-cleanup            Delete files after upload (default true)
```

## License

MIT

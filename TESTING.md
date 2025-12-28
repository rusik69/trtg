# Testing Guide

## Quick Test Options

### 1. Dry-Run Mode (Safest - No Downloads/Uploads)

Test the application without actually downloading or uploading anything:

```bash
# Build and run in dry-run mode
make dry-run

# Or manually:
go build -o bin/trtg .
./bin/trtg --dry-run
```

This will:
- Read torrents from `torrents.txt`
- Fetch torrent metadata (name, size)
- Show what would be downloaded
- **No actual downloads or uploads**

### 2. Test with a Small Public Torrent

Use a well-seeded, small test torrent:

1. **Add a test torrent to `torrents.txt`**:
   ```bash
   # Example: Ubuntu ISO (small, well-seeded)
   magnet:?xt=urn:btih:1234567890abcdef1234567890abcdef12345678&dn=ubuntu-22.04-desktop-amd64.iso
   
   # Or use a .torrent file:
   # /path/to/test.torrent
   ```

2. **Set up environment** (if testing locally without Docker):
   ```bash
   export TELEGRAM_BOT_TOKEN="your_bot_token"
   export TELEGRAM_CHAT_ID="your_chat_id"
   export TELEGRAM_API_URL="http://localhost:8081"
   export TELEGRAM_API_ID="your_api_id"
   export TELEGRAM_API_HASH="your_api_hash"
   ```

3. **Start Local Bot API Server** (if not using Docker):
   ```bash
   make run-local-api
   # Or manually:
   docker run -d -p 8081:8081 --name=telegram-bot-api \
     -e TELEGRAM_API_ID=${TELEGRAM_API_ID} \
     -e TELEGRAM_API_HASH=${TELEGRAM_API_HASH} \
     aiogram/telegram-bot-api:latest
   ```

4. **Run the application**:
   ```bash
   # Local (without Docker)
   make run
   
   # Or with Docker
   make docker-run
   ```

### 3. Test with Docker Compose (Recommended)

1. **Create `.env` file**:
   ```bash
   cp prod.env .env
   # Edit .env with your credentials
   ```

2. **Add test torrents to `torrents.txt`**:
   ```bash
   echo "magnet:?xt=urn:btih:..." >> torrents.txt
   ```

3. **Start services**:
   ```bash
   docker-compose up
   ```

4. **Run once** (if using `restart: "no"`):
   ```bash
   docker-compose run --rm trtg
   ```

### 4. Test Database Tracking

Verify that already-downloaded torrents are skipped:

1. **Run once** to download a torrent
2. **Run again** - it should skip the already-downloaded torrent:
   ```bash
   ./bin/trtg
   # Should show: "Skipping already downloaded torrent: ..."
   ```

3. **Check database**:
   ```bash
   docker exec -it trtg-postgres psql -U trtg -d trtg -c "SELECT * FROM videos;"
   # Or if running locally:
   # psql postgres://trtg:trtg@127.0.0.1:5432/trtg -c "SELECT * FROM videos;"
   ```

### 5. Test File Size Limits

Test that files larger than 2GB are skipped:

1. **Add a large torrent** to `torrents.txt`
2. **Run the application**
3. **Check logs** - should show:
   ```
   Warning: File ... is too large (X.XX GB > 2GB), skipping
   ```

## Test Scenarios

### Scenario 1: First Run (Empty Database)
```bash
# Clean start - truncate tables
docker exec -it trtg-postgres psql -U trtg -d trtg -c "TRUNCATE videos, channels CASCADE;"
# Or restart with fresh database
docker-compose down -v && docker-compose up -d

./bin/trtg --dry-run  # Preview
./bin/trtg             # Actual run
```

### Scenario 2: Duplicate Detection
```bash
# Run twice
./bin/trtg
./bin/trtg  # Should skip already downloaded
```

### Scenario 3: Multiple Files in Torrent
```bash
# Use a torrent with multiple files
# All files should be uploaded separately
./bin/trtg
```

### Scenario 4: Cleanup After Upload
```bash
# Run with cleanup enabled (default)
./bin/trtg --cleanup

# Run with cleanup disabled
./bin/trtg --cleanup=false
```

## Finding Test Torrents

### Small Test Torrents (for quick testing):
- **Ubuntu ISO**: Well-seeded, official torrents
- **Linux distributions**: Usually have official torrents
- **Public domain content**: Archive.org, etc.

### Magnet Links Format:
```
magnet:?xt=urn:btih:HASH&dn=NAME
```

Where:
- `HASH` is the torrent info hash (40 hex characters)
- `NAME` is the display name (optional)

### Example Test Torrents:
```bash
# Small Ubuntu ISO (replace with actual hash)
magnet:?xt=urn:btih:1234567890abcdef1234567890abcdef12345678&dn=ubuntu-test.iso

# Or use a .torrent file
/path/to/test.torrent
```

## Verification Checklist

After running, verify:

- [ ] Torrent metadata fetched successfully
- [ ] Files downloaded to `downloads/` directory
- [ ] Files uploaded to Telegram (check your chat)
- [ ] Database entry created in PostgreSQL
- [ ] Files cleaned up (if `--cleanup` enabled)
- [ ] Duplicate torrents are skipped on second run

## Troubleshooting

### Torrent won't download:
- Check if torrent has seeders
- Verify magnet link or .torrent file path is correct
- Check network connectivity

### Telegram upload fails:
- Verify bot token and chat ID
- Ensure Local Bot API Server is running
- Check file size (must be < 2GB)
- Start a conversation with your bot first

### Database issues:
- Verify PostgreSQL container is running: `docker ps | grep trtg-postgres`
- Check PostgreSQL connection: `docker exec -it trtg-postgres psql -U trtg -d trtg -c "\dt"`
- Check PostgreSQL logs: `docker logs trtg-postgres`

## Debug Mode

For more verbose output, check the logs:
```bash
# Docker logs
docker-compose logs -f trtg

# Or add debug logging to code
```

## Quick Test Script

```bash
#!/bin/bash
# quick-test.sh

echo "1. Testing dry-run..."
./bin/trtg --dry-run

echo ""
echo "2. Testing with small torrent..."
# Add a small test torrent to torrents.txt first
./bin/trtg --cleanup=false

echo ""
echo "3. Testing duplicate detection..."
./bin/trtg  # Should skip

echo ""
echo "4. Checking database..."
docker exec -it trtg-postgres psql -U trtg -d trtg -c "SELECT video_id, title, uploaded_at FROM videos;"
```

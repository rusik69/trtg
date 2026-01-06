# Database Tools - Fix Episode Categorization

## Problem

Most episodes were not being categorized by season because the parser didn't support all naming formats. Only Season 28 was properly categorized because it happened to use a format the parser could handle.

## Solution

The parser has been improved to handle more naming formats including:
- `SxxExx` format (e.g., S01E01, S28E15)
- `XxY` format (e.g., 10x05, 5x12)
- `Season X` folder structures
- `Episode X` patterns

Additionally, two new tools have been created to diagnose and fix the categorization issue:

### 1. Diagnose Tool

Shows the current categorization status and samples of uncategorized videos.

**Local usage:**
```bash
make diagnose
```

**Remote server usage:**
```bash
make deploy-diagnose
```

This will show:
- Current season distribution
- Sample uncategorized files
- What the improved parser would detect
- Estimated fixable count

### 2. Reparse Tool

Re-parses all videos in the database and updates their season/episode information.

**Dry run (preview changes without applying them):**
```bash
# Local
make reparse-dry-run

# Remote server
make deploy-reparse-dry-run
```

**Apply changes:**
```bash
# Local
make reparse

# Remote server
make deploy-reparse
```

## Recommended Steps

1. **First, diagnose the issue** to see what will be fixed:
   ```bash
   make deploy-diagnose
   ```

2. **Preview the changes** without applying them:
   ```bash
   make deploy-reparse-dry-run
   ```

3. **Apply the fix** if the preview looks good:
   ```bash
   make deploy-reparse
   ```

4. **Verify the fix** by checking the web interface - episodes should now be properly organized by season.

## What Gets Updated

The reparse tool updates three fields for each video:
- `show_name` - Extracted from the torrent name or file path
- `season_number` - Detected from file path or folder structure (0 = uncategorized)
- `episode_number` - Detected from file path (0 = unknown)

## Safety

- The reparse tool only updates these metadata fields
- No files are deleted or modified
- Telegram upload status is preserved
- Database backups are recommended before running (but the operation is safe)

## Technical Details

### Parser Improvements (pkg/parser/parser.go)

Added support for:
1. `XxY` format (e.g., `simpsons_10x05.mkv`)
2. Better folder structure detection
3. Improved show name extraction from multilingual torrents

### New Commands (cmd/)

- `cmd/diagnose` - Diagnostic tool to check categorization status
- `cmd/reparse` - Migration tool to update all video metadata

### Makefile Targets

Local:
- `make diagnose` - Run diagnostic locally
- `make reparse-dry-run` - Preview changes locally
- `make reparse` - Apply changes locally

Remote:
- `make deploy-diagnose` - Run diagnostic on server
- `make deploy-reparse-dry-run` - Preview changes on server
- `make deploy-reparse` - Apply changes on server

## Example Output

### Diagnose

```
Total videos in database: 790

=== Current Season Distribution ===
Uncategorized (Season 0): 762 episodes
Season 28: 28 episodes

=== Sample Uncategorized Files (showing 20) ===

[1] File: Season 1/The.Simpsons.S01E01.mkv
    Current: Show='', Season=0, Episode=0
    Re-parsed: Show='The Simpsons', Season=1, Episode=1
    ✓ Can be fixed!

[2] File: Season 5/The.Simpsons.S05E10.720p.WEB-DL.mkv
    Current: Show='', Season=0, Episode=0
    Re-parsed: Show='The Simpsons', Season=5, Episode=10
    ✓ Can be fixed!
...

=== Summary ===
Total uncategorized: 762
Fixable (from samples): 20/20
```

### Reparse (Dry Run)

```
Found 790 videos in database
DRY RUN MODE - no changes will be made

[1/790] UPDATE: Season 1/The.Simpsons.S01E01.mkv
  Old: Show='', Season=0, Episode=0
  New: Show='The Simpsons', Season=1, Episode=1
  (would update)

[2/790] UPDATE: Season 1/The.Simpsons.S01E02.mkv
  Old: Show='', Season=0, Episode=0
  New: Show='The Simpsons', Season=1, Episode=2
  (would update)
...

=== Summary ===
Total videos: 790
Updated: 762
Unchanged: 28

=== Season Distribution (after re-parsing) ===
Season 1: 22 episodes
Season 2: 22 episodes
Season 3: 24 episodes
...
Season 36: 20 episodes
```

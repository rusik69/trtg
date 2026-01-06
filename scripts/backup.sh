#!/bin/sh
set -e

# Configuration
BACKUP_DIR="/backups"
POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_USER="${POSTGRES_USER:-trtg}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trtg}"
POSTGRES_DB="${POSTGRES_DB:-trtg}"
BACKUP_KEEP_DAYS="${BACKUP_KEEP_DAYS:-7}"

# Create backup directory if it doesn't exist
mkdir -p "$BACKUP_DIR"

# Generate backup filename with timestamp
BACKUP_FILE="$BACKUP_DIR/trtg_backup_$(date +%Y%m%d_%H%M%S).sql.gz"

echo "$(date +%Y-%m-%d\ %H:%M:%S) - Starting database backup..."

# Create backup using pg_dump and compress with gzip
export PGPASSWORD="$POSTGRES_PASSWORD"
pg_dump -h "$POSTGRES_HOST" \
        -p "$POSTGRES_PORT" \
        -U "$POSTGRES_USER" \
        -d "$POSTGRES_DB" \
        --format=plain \
        --no-owner \
        --no-acl \
        | gzip > "$BACKUP_FILE"

# Check if backup was successful
if [ $? -eq 0 ]; then
    BACKUP_SIZE=$(du -h "$BACKUP_FILE" | cut -f1)
    echo "$(date +%Y-%m-%d\ %H:%M:%S) - Backup completed successfully: $BACKUP_FILE ($BACKUP_SIZE)"
else
    echo "$(date +%Y-%m-%d\ %H:%M:%S) - Backup failed!"
    exit 1
fi

# Remove backups older than BACKUP_KEEP_DAYS
echo "$(date +%Y-%m-%d\ %H:%M:%S) - Removing backups older than $BACKUP_KEEP_DAYS days..."
find "$BACKUP_DIR" -name "trtg_backup_*.sql.gz" -type f -mtime +$BACKUP_KEEP_DAYS -delete

# Count remaining backups
BACKUP_COUNT=$(find "$BACKUP_DIR" -name "trtg_backup_*.sql.gz" -type f | wc -l)
echo "$(date +%Y-%m-%d\ %H:%M:%S) - Total backups: $BACKUP_COUNT"
echo "$(date +%Y-%m-%d\ %H:%M:%S) - Backup process finished"

# Database Backup System

This project includes an automated database backup system that stores backups in a host directory, making them safe from Docker volume removal.

## Overview

- **Backup Location**: `./backups/` directory on the host machine
- **Schedule**: Every hour (configurable via cron)
- **Retention**: Last 1 day of backups (configurable via `BACKUP_KEEP_DAYS`)
- **Format**: Compressed SQL dumps (`.sql.gz`)
- **Naming**: `trtg_backup_YYYYMMDD_HHMMSS.sql.gz`

## Architecture

The backup system consists of:

1. **postgres-backup service**: A sidecar container that runs periodic backups
2. **backup.sh script**: Shell script that performs the actual backup using `pg_dump`
3. **Host directory mount**: `./backups/` is mounted to both postgres and backup containers
4. **Cron scheduler**: Runs backups on a schedule

## Configuration

Environment variables in `docker-compose.yml`:

```yaml
POSTGRES_USER: trtg
POSTGRES_PASSWORD: trtg
POSTGRES_DB: trtg
POSTGRES_HOST: 127.0.0.1
BACKUP_KEEP_DAYS: 1  # Number of days to retain backups
```

## Usage

### Manual Backup

Trigger a backup manually on the remote server:

```bash
make deploy-backup
```

### List Backups

List all available backups:

```bash
make deploy-backup-list
```

### Download Latest Backup

Download the most recent backup to your local machine:

```bash
make deploy-backup-download
```

### Restore from Backup

Restore the database from a backup file:

```bash
make deploy-backup-restore BACKUP_FILE=backups/trtg_backup_20240101_120000.sql.gz
```

**Warning**: This will overwrite the current database!

## Automatic Backups

Backups run automatically every hour. The schedule can be modified in `docker-compose.yml`:

```yaml
# Change this line to adjust the schedule (cron format)
echo "0 * * * * /backup.sh >> /var/log/backup.log 2>&1" > /etc/crontabs/root
```

Cron format: `minute hour day month weekday`

Examples:
- `0 * * * *` - Every hour (current setting)
- `0 */6 * * *` - Every 6 hours
- `0 2 * * *` - Daily at 2:00 AM
- `0 0 * * 0` - Weekly on Sunday at midnight

## Backup Retention

Old backups are automatically cleaned up based on `BACKUP_KEEP_DAYS`. To change retention:

1. Update `BACKUP_KEEP_DAYS` in `docker-compose.yml`
2. Redeploy: `make deploy`

## Monitoring

View backup logs:

```bash
ssh ubuntu@hetzner.govno2.cloud "sudo docker logs trtg-postgres-backup"
```

## Recovery Procedure

1. Download the backup you want to restore:
   ```bash
   make deploy-backup-download
   ```

2. Verify the backup file exists locally:
   ```bash
   ls -lh backups/
   ```

3. Restore the database:
   ```bash
   make deploy-backup-restore BACKUP_FILE=backups/trtg_backup_YYYYMMDD_HHMMSS.sql.gz
   ```

## Troubleshooting

### No backups found

Check if the postgres-backup container is running:
```bash
ssh ubuntu@hetzner.govno2.cloud "sudo docker ps | grep postgres-backup"
```

Check backup logs:
```bash
ssh ubuntu@hetzner.govno2.cloud "sudo docker logs trtg-postgres-backup"
```

### Backup failed

Ensure PostgreSQL is healthy:
```bash
ssh ubuntu@hetzner.govno2.cloud "sudo docker exec trtg-postgres pg_isready -U trtg"
```

Check disk space:
```bash
ssh ubuntu@hetzner.govno2.cloud "df -h /opt/trtg/backups"
```

### Manual backup from inside container

```bash
ssh ubuntu@hetzner.govno2.cloud
cd /opt/trtg
sudo docker exec trtg-postgres-backup /backup.sh
```

## File Locations

- **Host backup directory**: `/opt/trtg/backups/` (on remote server)
- **Container backup directory**: `/backups/` (inside containers)
- **Backup script**: `./scripts/backup.sh`
- **Local backup directory**: `./backups/` (on your local machine, gitignored)

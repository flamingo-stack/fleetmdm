#!/bin/sh
set -e

CONFIG_FILE="${FLEET_CONFIG:-/etc/fleet/fleet.yml}"

# Parse MySQL connection (default port 3306)
MYSQL_HOST="${FLEET_MYSQL_ADDRESS%%:*}"
MYSQL_PORT="${FLEET_MYSQL_ADDRESS##*:}"
[ "$MYSQL_PORT" = "$MYSQL_HOST" ] && MYSQL_PORT=3306

# Parse Redis connection (default port 6379)
REDIS_HOST="${FLEET_REDIS_ADDRESS%%:*}"
REDIS_PORT="${FLEET_REDIS_ADDRESS##*:}"
[ "$REDIS_PORT" = "$REDIS_HOST" ] && REDIS_PORT=6379

# Wait for MySQL
echo "Waiting for MySQL ($MYSQL_HOST:$MYSQL_PORT)..."
while ! nc -z "$MYSQL_HOST" "$MYSQL_PORT" 2>/dev/null; do
    sleep 2
done
echo "MySQL is ready"

# Wait for Redis
echo "Waiting for Redis ($REDIS_HOST:$REDIS_PORT)..."
while ! nc -z "$REDIS_HOST" "$REDIS_PORT" 2>/dev/null; do
    sleep 2
done
echo "Redis is ready"

# Prepare database
echo "Preparing database..."
fleet prepare db --config "$CONFIG_FILE" --no-prompt

# Start Fleet
echo "Starting Fleet server..."
exec fleet serve --config "$CONFIG_FILE"

#!/bin/sh
set -e

CONFIG_FILE="${FLEET_CONFIG:-/etc/fleet/fleet.yml}"

# Parse MySQL connection
MYSQL_HOST=$(echo "$FLEET_MYSQL_ADDRESS" | cut -d':' -f1)
MYSQL_PORT=$(echo "$FLEET_MYSQL_ADDRESS" | cut -d':' -f2)

# Parse Redis connection
REDIS_HOST=$(echo "$FLEET_REDIS_ADDRESS" | cut -d':' -f1)
REDIS_PORT=$(echo "$FLEET_REDIS_ADDRESS" | cut -d':' -f2)

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

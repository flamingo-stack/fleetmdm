#!/bin/sh
set -e

CONFIG_FILE="${FLEET_CONFIG:-/etc/fleet/fleet.yml}"
FLEET_SERVER_PORT="${FLEET_SERVER_PORT:-8070}"

# Validate required environment variables
if [ -z "$FLEET_MYSQL_ADDRESS" ]; then
    echo "Error: FLEET_MYSQL_ADDRESS environment variable is required" >&2
    exit 1
fi

if [ -z "$FLEET_REDIS_ADDRESS" ]; then
    echo "Error: FLEET_REDIS_ADDRESS environment variable is required" >&2
    exit 1
fi

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

# Function to wait for Fleet to be ready
wait_for_fleet() {
    echo "Waiting for Fleet to be ready..."
    attempts=0
    max_attempts=30

    while [ $attempts -lt $max_attempts ]; do
        if curl -sf "http://localhost:${FLEET_SERVER_PORT}/healthz" >/dev/null 2>&1; then
            echo "Fleet is ready!"
            return 0
        fi
        attempts=$((attempts + 1))
        sleep 5
    done

    echo "Fleet failed to start after $max_attempts attempts"
    return 1
}

# Start Fleet server
echo "Starting Fleet server..."
fleet serve --config "$CONFIG_FILE" &
FLEET_PID=$!

# Wait for Fleet to be ready
wait_for_fleet

# Initialize Fleet if auto-init is enabled and API token doesn't exist
if [ "$FLEET_SETUP_AUTO_INIT" = "true" ] && [ ! -f /etc/fleet/api_token.txt ]; then
    echo "Running Fleet initialization..."
    sh /usr/share/init-fleet.sh || true
fi

# Keep container running and monitor Fleet process
wait $FLEET_PID

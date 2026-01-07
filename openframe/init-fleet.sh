#!/bin/sh
set -e

FLEET_SERVER_PORT="${FLEET_SERVER_PORT:-8070}"

# Check if API token file exists at the start
if [ -f /etc/fleet/api_token.txt ]; then
    echo "API token file already exists, skipping initialization..."
    exit 0
fi

# Configure fleetctl
echo "Configuring fleetctl..."
fleetctl config set --address "http://localhost:${FLEET_SERVER_PORT}"

# Function to check if Fleet is already initialized
check_fleet_initialized() {
    if fleetctl login --email="${FLEET_SETUP_ADMIN_EMAIL}" --password="${FLEET_SETUP_ADMIN_PASSWORD}" >/dev/null 2>&1; then
        return 0
    fi
    return 1
}

# Initialize Fleet if not already done
if ! check_fleet_initialized; then
    echo "Performing initial Fleet setup..."
    fleetctl setup --email="${FLEET_SETUP_ADMIN_EMAIL}" \
                  --password="${FLEET_SETUP_ADMIN_PASSWORD}" \
                  --org-name="${FLEET_SETUP_ORG_NAME}" \
                  --name="${FLEET_SETUP_ADMIN_NAME:-Admin}" || true
else
    echo "Fleet already initialized, skipping setup"
fi

# Login as admin
echo "Logging in as admin..."
fleetctl login --email="${FLEET_SETUP_ADMIN_EMAIL}" --password="${FLEET_SETUP_ADMIN_PASSWORD}"

# Try to log in as API user first
echo "Attempting to log in as API user..."
if fleetctl login --email="${FLEET_API_USER_EMAIL}" --password="${FLEET_API_USER_PASSWORD}" >/dev/null 2>&1; then
    echo "Successfully logged in as API user"
    TOKEN=$(grep "token:" ~/.fleet/config 2>/dev/null | awk '{print $2}')
    if [ -n "$TOKEN" ]; then
        echo "Successfully got API token"
        echo "$TOKEN" > /etc/fleet/api_token.txt
        chmod 600 /etc/fleet/api_token.txt
        echo "API token saved to /etc/fleet/api_token.txt"
    else
        echo "Failed to get API token"
        exit 1
    fi
else
    # Create API-only user if login failed
    echo "API user doesn't exist, creating..."
    API_USER_OUTPUT=$(fleetctl user create \
        --name "${FLEET_API_USER_NAME}" \
        --email "${FLEET_API_USER_EMAIL}" \
        --password "${FLEET_API_USER_PASSWORD}" \
        --global-role admin \
        --api-only 2>&1) || true

    # Extract token from output
    if echo "$API_USER_OUTPUT" | grep -q "Success! The API token for your new user is:"; then
        TOKEN=$(echo "$API_USER_OUTPUT" | grep "Success! The API token for your new user is:" | awk -F': ' '{print $2}')
        echo "Successfully created API user and got token"
        echo "$TOKEN" > /etc/fleet/api_token.txt
        chmod 600 /etc/fleet/api_token.txt
        echo "API token saved to /etc/fleet/api_token.txt"

        # Configure fleetctl to use the API token
        fleetctl config set --token "$TOKEN"
        echo "fleetctl configured with API token"
    else
        echo "Failed to create API user. Output was:"
        echo "$API_USER_OUTPUT"
        exit 1
    fi
fi

echo "Fleet initialization complete!"

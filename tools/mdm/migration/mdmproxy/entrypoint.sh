#!/bin/sh

set -e 

if [ -z "${MDMPROXY_SERVER_ADDRESS}" ]; then
	MDMPROXY_SERVER_ADDRESS=":8080"
fi

set -- \
	-existing-hostname "${MDMPROXY_EXISTING_HOSTNAME:?}" \
	-existing-url "${MDMPROXY_EXISTING_URL:?}" \
	-fleet-url "${MDMPROXY_FLEET_URL:?}" \
	-server-address "${MDMPROXY_SERVER_ADDRESS:?}"

if [ -n "${MDMPROXY_AUTH_TOKEN}" ]; then
	set -- "$@" -auth-token "${MDMPROXY_AUTH_TOKEN:?}"
fi

if [ -n "${MDMPROXY_MIGRATE_PERCENTAGE}" ]; then
	set -- "$@" -migrate-percentage "${MDMPROXY_MIGRATE_PERCENTAGE:?}"
fi

if [ -n "${MDMPROXY_MIGRATE_UDIDS}" ]; then
	set -- "$@" -migrate-udids "${MDMPROXY_MIGRATE_UDIDS:?}"
fi

exec /usr/bin/mdmproxy "$@"

ARG FLEET_VERSION=4.78.0

# Stage 1: Builder - install fleetctl
FROM alpine:3.21 AS builder

ARG FLEET_VERSION
RUN apk add --no-cache nodejs npm && \
    npm install -g fleetctl@${FLEET_VERSION}

# Stage 2: Runtime
FROM fleetdm/fleet:v${FLEET_VERSION}

LABEL org.opencontainers.image.description="OpenFrame Fleet MDM Server"
LABEL org.opencontainers.image.source="https://github.com/flamingo-stack/fleet"

USER root

ARG FLEET_SERVER_PORT=8080
ENV FLEET_SERVER_PORT=${FLEET_SERVER_PORT}

RUN mkdir -p /etc/fleet /var/log/osquery && \
    apk add --no-cache curl netcat-openbsd nodejs

# Copy fleetctl from builder
COPY --from=builder /usr/local/lib/node_modules /usr/local/lib/node_modules
COPY --from=builder /usr/local/bin/fleetctl /usr/local/bin/fleetctl

# Copy configuration files
COPY entrypoint.sh /entrypoint.sh
COPY fleet.yml /etc/fleet/fleet.yml

RUN chmod +x /entrypoint.sh

EXPOSE ${FLEET_SERVER_PORT}

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD ["sh", "-c", "curl -sf http://localhost:${FLEET_SERVER_PORT:-8080}/healthz || exit 1"]

ENTRYPOINT ["/entrypoint.sh"]

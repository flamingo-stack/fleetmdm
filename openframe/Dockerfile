ARG FLEET_VERSION=4.78.0

# Stage 1: Builder - download fleetctl binary from GitHub releases
FROM alpine:3.21 AS builder

ARG FLEET_VERSION
ARG TARGETARCH

RUN apk add --no-cache curl tar && \
    ARCH=$([ "$TARGETARCH" = "arm64" ] && echo "arm64" || echo "amd64") && \
    curl -fsSL "https://github.com/fleetdm/fleet/releases/download/fleet-v${FLEET_VERSION}/fleetctl_v${FLEET_VERSION}_linux_${ARCH}.tar.gz" \
    | tar -xzf - -C /usr/local/bin fleetctl

# Stage 2: Runtime
FROM fleetdm/fleet:v${FLEET_VERSION}

LABEL org.opencontainers.image.description="OpenFrame Fleet MDM Server"
LABEL org.opencontainers.image.source="https://github.com/flamingo-stack/fleet"

USER root

ARG FLEET_SERVER_PORT=8080
ENV FLEET_SERVER_PORT=${FLEET_SERVER_PORT}
ENV FLEET_SERVER_ADDRESS=0.0.0.0:${FLEET_SERVER_PORT}

RUN mkdir -p /etc/fleet /var/log/osquery && \
    apk add --no-cache curl netcat-openbsd

# Copy fleetctl from builder
COPY --from=builder /usr/local/bin/fleetctl /usr/local/bin/fleetctl

# Copy configuration files
COPY entrypoint.sh /entrypoint.sh
COPY fleet.yml /etc/fleet/fleet.yml

RUN chmod +x /entrypoint.sh

EXPOSE ${FLEET_SERVER_PORT}

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD ["sh", "-c", "curl -sf http://localhost:${FLEET_SERVER_PORT:-8080}/healthz || exit 1"]

ENTRYPOINT ["/entrypoint.sh"]

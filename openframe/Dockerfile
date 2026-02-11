ARG FLEET_VERSION=4.78.0

# Stage 1: Builder - download fleetctl binary from GitHub releases
FROM alpine:3.21 AS builder

ARG FLEET_VERSION
ARG TARGETARCH

RUN apk add --no-cache curl tar && \
    ARCH=$([ "$TARGETARCH" = "arm64" ] && echo "arm64" || echo "amd64") && \
    curl -fsSL "https://github.com/fleetdm/fleet/releases/download/fleet-v${FLEET_VERSION}/fleetctl_v${FLEET_VERSION}_linux_${ARCH}.tar.gz" \
    | tar -xzf - --strip-components=1 -C /usr/local/bin "fleetctl_v${FLEET_VERSION}_linux_${ARCH}/fleetctl"

# Stage 2: Runtime
FROM fleetdm/fleet:v${FLEET_VERSION}

LABEL org.opencontainers.image.description="OpenFrame Fleet MDM Server"
LABEL org.opencontainers.image.source="https://github.com/flamingo-stack/fleet"

USER root

# Server port - matches K8s service configuration
ARG FLEET_SERVER_PORT=8070
ENV FLEET_SERVER_PORT=${FLEET_SERVER_PORT}

RUN mkdir -p /etc/fleet /var/log/osquery && \
    chmod 755 /var/log/osquery && \
    apk add --no-cache curl netcat-openbsd

# Copy fleetctl from builder
COPY --from=builder /usr/local/bin/fleetctl /usr/local/bin/fleetctl

# Copy configuration files
COPY entrypoint.sh /entrypoint.sh
COPY init-fleet.sh /usr/share/init-fleet.sh
COPY fleet.yml /etc/fleet/fleet.yml

RUN chmod +x /entrypoint.sh /usr/share/init-fleet.sh

EXPOSE ${FLEET_SERVER_PORT}

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD curl -sf http://localhost:${FLEET_SERVER_PORT}/healthz || exit 1

ENTRYPOINT ["/entrypoint.sh"]

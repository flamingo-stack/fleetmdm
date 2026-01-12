FROM alpine:3.23.0@sha256:51183f2cfa6320055da30872f211093f9ff1d3cf06f39a0bdb212314c5dc7375
LABEL maintainer="Fleet Developers"

# Install dependencies
RUN apk --update add ca-certificates && \
    apk --no-cache add jq bind-tools curl iputils netcat-openbsd bash && \
    rm -rf /var/cache/apk/*

# Create directories
RUN mkdir -p /usr/share /etc/fleet /var/log/osquery && \
    chmod 755 /var/log/osquery

# Create fleet group and user
RUN addgroup -S fleet && adduser -S fleet -G fleet

# Copy binaries
COPY fleet /usr/bin/
COPY fleetctl /usr/bin/

# Copy initialization scripts and config
COPY entrypoint.sh /entrypoint.sh
COPY init-fleet.sh /usr/share/init-fleet.sh
COPY fleet.yml /etc/fleet/fleet.yml

# Set permissions
RUN chmod +x /entrypoint.sh /usr/share/init-fleet.sh /usr/bin/fleet /usr/bin/fleetctl

# Default environment variables
ENV FLEET_CONFIG=/etc/fleet/fleet.yml \
    FLEET_SERVER_PORT=8070

EXPOSE 8070

# Healthcheck
HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD curl -f http://localhost:${FLEET_SERVER_PORT}/healthz || exit 1

ENTRYPOINT ["/entrypoint.sh"]

FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
LABEL maintainer="Fleet Developers"

RUN apk upgrade --no-cache \
    && apk add --no-cache ca-certificates \
    && addgroup -g 3333 -S fleet \
    && adduser -u 3333 -S fleet -G fleet

USER fleet

COPY fleet /usr/bin/

CMD ["fleet", "serve"]


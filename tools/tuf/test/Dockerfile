FROM debian:trixie-slim@sha256:28de0877c2189802884ccd20f15ee41c203573bd87bb6b883f5f46362d24c5c2

WORKDIR /usr/src/app

COPY test_tuf file-server ./

CMD ["./file-server", "8081", "repository"]

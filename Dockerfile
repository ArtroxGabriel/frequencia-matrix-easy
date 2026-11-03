# syntax=docker/dockerfile:1.7
ARG GO_VERSION=alpine

# Build stage
FROM golang:${GO_VERSION} AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata

RUN addgroup -g 10001 -S scratchgroup && \
    adduser -u 10001 -S -G scratchgroup -h /app scratchuser

RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,source=go.sum,target=go.sum \
    --mount=type=bind,source=go.mod,target=go.mod \
    go mod download -x

RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,target=. \
    CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w -extldflags '-static'" \
    -o /bin/server \
    ./cmd/server

RUN mkdir -p /app/data && chown -R 10001:10001 /app

# Final runtime image
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group
COPY --from=builder --chown=10001:10001 /app /app
COPY --from=builder /bin/server /server

USER 10001:10001
WORKDIR /app

ENV PORT=8080 \
    DB_PATH=/app/data/frequencia.db

EXPOSE 8080

ENTRYPOINT ["/server"]

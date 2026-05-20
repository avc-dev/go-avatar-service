# syntax=docker/dockerfile:1.7

# --- Builder ---------------------------------------------------------------
# golang:1.25-alpine matches the host toolchain. CGO is disabled so the
# resulting binaries are fully static and run on a scratch/alpine base.
FROM golang:1.25-alpine AS builder

WORKDIR /src

# Cache deps in a separate layer so source-only changes don't re-download.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Both binaries from the same builder for layer/cache reuse. -s -w strip the
# symbol table for smaller images; the embedded BuildKit cache mount speeds
# up rebuilds on the same host.
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/server ./cmd/server
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/worker ./cmd/worker

# --- Runtime ---------------------------------------------------------------
FROM alpine:3.20

# ca-certificates for HTTPS to MinIO/AWS S3; tzdata for correct timestamps.
# Busybox wget (already present) is used by the compose healthcheck.
RUN apk --no-cache add ca-certificates tzdata

# Non-root runtime user. The binaries don't need to bind to privileged ports
# and don't write to the filesystem, so a plain UID is sufficient.
RUN addgroup -g 1000 -S app && adduser -u 1000 -S -G app app

WORKDIR /app

COPY --from=builder /out/server /usr/local/bin/server
COPY --from=builder /out/worker /usr/local/bin/worker
COPY --from=builder /src/web /app/web

USER app

# Default to the HTTP server. Use CMD (not ENTRYPOINT) so compose's `command:`
# directive can fully replace the launched binary — ENTRYPOINT would only
# pass `command` as argv to the fixed entrypoint, which is not what we want
# for a "two binaries, one image" layout.
CMD ["/usr/local/bin/server"]

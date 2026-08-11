FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app
COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server ./cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates curl tzdata docker-cli

COPY --from=builder /server /usr/local/bin/server
# Bake session Dockerfiles into the API image so Dokploy does not depend on bind mounts.
COPY desktop-container/ /build/desktop-container/
COPY relay-container/ /build/relay-container/

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=20s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["server"]

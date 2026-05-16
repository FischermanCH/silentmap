# ── Build Stage ──────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache \
    git \
    libpcap-dev \
    gcc \
    musl-dev

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION:-dev} -X main.commit=${COMMIT:-unknown}" \
    -o silentmap \
    ./cmd/silentmap

# ── Runtime Stage ─────────────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache \
    libpcap \
    ca-certificates \
    tzdata

# Nicht als root laufen — aber NET_RAW Cap wird vom Host gesetzt
RUN addgroup -S silentmap && adduser -S -G silentmap silentmap

WORKDIR /app

COPY --from=builder /build/silentmap .

# Data-Volume für SQLite, Config, Modelle
VOLUME ["/data"]

# Web UI
EXPOSE 8080

# Healthcheck
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

USER silentmap

ENTRYPOINT ["/app/silentmap"]
CMD ["--data", "/data"]

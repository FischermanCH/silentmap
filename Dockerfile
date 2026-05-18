# --- Build stage ---
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o silentmap \
    ./cmd/silentmap

# --- Runtime stage ---
FROM alpine:3.20

RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    nmap \
    iputils

RUN apk add --no-cache libcap && \
    addgroup -S silentmap && adduser -S -G silentmap silentmap

WORKDIR /app

COPY --from=builder /build/silentmap .

RUN setcap 'cap_net_raw+eip' /app/silentmap

VOLUME ["/data"]

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

USER silentmap

ENTRYPOINT ["/app/silentmap"]
CMD ["--data", "/data"]

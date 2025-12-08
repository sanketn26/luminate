# Multi-stage build for minimal image size
FROM golang:1.21-alpine AS builder

# Build arguments
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME} -w -s" \
    -o luminate \
    ./cmd/luminate

# Final stage - minimal runtime image
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 luminate && \
    adduser -D -u 1000 -G luminate luminate

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/luminate .

# Copy default configuration
COPY configs/config.yaml ./configs/

# Create data directory
RUN mkdir -p /var/lib/luminate && \
    chown -R luminate:luminate /var/lib/luminate /app

# Switch to non-root user
USER luminate

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/v1/health || exit 1

# Volume for data
VOLUME ["/var/lib/luminate"]

# Run the application
ENTRYPOINT ["./luminate"]
CMD ["--config", "configs/config.yaml"]

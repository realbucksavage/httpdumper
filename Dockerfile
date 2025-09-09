# Multi-stage build for httpdumper
# Build stage
FROM golang:1.24-alpine AS build

WORKDIR /app

# Enable Go modules and disable CGO for a static binary
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

# Pre-cache module dependencies
COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Copy source
COPY . .

# Build the binary
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags="-s -w" -o /out/httpdumper .

# Runtime stage (distroless static for minimal image)
FROM gcr.io/distroless/static:nonroot

# Expose default port used by the app (can be overridden with -port flag)
EXPOSE 8080

# Copy binary from builder
COPY --from=build /out/httpdumper /usr/local/bin/httpdumper

USER nonroot:nonroot

# Default command; port can be changed at runtime: `-port=9090`
ENTRYPOINT ["/usr/local/bin/httpdumper"]
CMD ["-port=8080"]

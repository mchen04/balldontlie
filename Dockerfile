# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies for SQLite
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with CGO enabled for SQLite
RUN CGO_ENABLED=1 GOOS=linux go build -o bot ./cmd/bot

# Runtime stage
FROM alpine:latest

# Install SQLite runtime dependencies
RUN apk add --no-cache ca-certificates sqlite-libs

WORKDIR /app

# Copy the binary
COPY --from=builder /app/bot .

# Create data directory
RUN mkdir -p /data

# Run as non-root user
RUN adduser -D -g '' appuser
RUN chown -R appuser:appuser /app /data
USER appuser

EXPOSE 8080

CMD ["./bot"]

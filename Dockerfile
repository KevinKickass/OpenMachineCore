# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git make

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN make build

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/bin/openmachinecore .
COPY --from=builder /app/configs ./configs

# Create non-root user
RUN addgroup -g 1000 openmachine && \
    adduser -D -u 1000 -G openmachine openmachine && \
    chown -R openmachine:openmachine /app

USER openmachine

EXPOSE 8080 50051

CMD ["./openmachinecore"]

# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy source
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o buildorbail .

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/buildorbail .
COPY --from=builder /app/web ./web
COPY --from=builder /app/internal/ai/prompts ./internal/ai/prompts

# Create non-root user
RUN adduser -D -g '' appuser
USER appuser

EXPOSE 5897

ENV PORT=5897
ENV GIN_MODE=release

CMD ["./buildorbail"]

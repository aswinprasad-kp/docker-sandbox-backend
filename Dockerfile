# STAGE 1: Build the Go application
FROM golang:1.26-alpine AS builder
WORKDIR /workspace

# 1. Copy shared Protobuf Contracts
COPY shared/go.mod shared/go.sum* ./shared/
COPY backend/go.mod backend/go.sum* ./backend/

# @. Download dependencies (Docker caches this step unless go.mod changes!)
WORKDIR /workspace/backend
RUN go mod download

# 3. Now copy the actual source code (after dependencies are cached)
WORKDIR /workspace
COPY shared/ ./shared/
COPY backend/ ./backend/

# 4. Compile the Go application into a binary named 'api-server'
WORKDIR /workspace/backend
RUN go build -o api-server .

# STAGE 2: The Production Image
FROM alpine:latest
WORKDIR /root/

# Copy the compiled binary from Stage 1
COPY --from=builder /workspace/backend/api-server .

COPY shared/database/migrations/ ./migrations/

EXPOSE 8080
CMD ["./api-server"]
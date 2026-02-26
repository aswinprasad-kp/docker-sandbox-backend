# STAGE 1: Build the Go application
FROM golang:1.26-alpine AS builder
WORKDIR /app

# Copy the module files first to cache dependencies
COPY go.mod go.sum* ./
RUN go mod download

# Copy ALL .go files (main.go, db.go, handlers.go)
COPY . .

# Compile the Go application into a binary named 'api-server'
RUN go build -o api-server .

# STAGE 2: The Production Image
FROM alpine:latest
WORKDIR /root/

# Copy the compiled binary from Stage 1
COPY --from=builder /app/api-server .

EXPOSE 8080
CMD ["./api-server"]
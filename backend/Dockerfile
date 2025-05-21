# Dockerfile: Docker configuration for CloudPulse backend


# Use the official Go image to build the app
FROM golang:1.21 AS builder

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum to download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the app
RUN CGO_ENABLED=0 GOOS=linux go build -o cloudpulse main.go

# Use a lightweight image for the runtime
FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/cloudpulse .
COPY --from=builder /app/frontend ./frontend
EXPOSE 8080
CMD ["./cloudpulse"]
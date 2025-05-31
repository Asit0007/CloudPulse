# Stage 1: Build the Go application
FROM golang:1.23-alpine AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download && go mod verify

# Copy the source code into the container
COPY . .

# Build the Go app.
# CGO_ENABLED=0 to build a statically-linked binary (no external C dependencies)
# -ldflags="-s -w" to strip debug information and symbols, reducing binary size
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /cloudpulse .

# Stage 2: Create the final, small image
FROM alpine:latest

# Install ca-certificates to ensure HTTPS calls (e.g., to AWS, GitHub, Vault) work
RUN apk --no-cache add ca-certificates

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy the built binary from the builder stage
COPY --from=builder /cloudpulse /cloudpulse

# Copy the frontend static assets. The Go app will serve these.
COPY ../frontend ./frontend
# Ensure the frontend directory exists in the image if it's empty in the build context
RUN mkdir -p ./frontend

# Expose port 8080 to the outside world (the port the Go app listens on)
EXPOSE 8080

# Environment variables that need to be set when running the container.
# These are placeholders; actual values will be injected during 'docker run' or by orchestration.
# ENV VAULT_ADDR="http://127.0.0.1:8200" # Example: if Vault is on the same Docker host network
# ENV VAULT_TOKEN=""                    # CRITICAL: Must be provided at runtime
# ENV GITHUB_OWNER=""                   # Your GitHub username or organization
# ENV GITHUB_REPO=""                    # Your GitHub repository name
# ENV PORT="8080"                       # Port for the backend to listen on
# ENV AWS_REGION="your-aws-region"      # e.g., us-east-1. SDK will pick this up.
# ENV EC2_INSTANCE_ID_OVERRIDE=""       # Optional: for local testing if not on EC2

# Command to run the executable
CMD ["/cloudpulse"]

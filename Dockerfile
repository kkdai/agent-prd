# Use the official Golang image to create a build artifact.
# This is the "builder" stage.
FROM golang:1.24-alpine AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source code into the container
COPY . .

# Build the Go app
# -o /app/server: specifies the output file name
# CGO_ENABLED=0: disables Cgo to create a statically linked binary
# GOOS=linux: specifies the target operating system as Linux
RUN CGO_ENABLED=0 GOOS=linux go build -v -o /app/server .

# ---
# Start a new stage for the runtime environment.
# We use a Node.js base image to get 'npm' and install 'git'.
FROM node:lts-alpine

# Install git and the Gemini CLI globally
RUN apk add --no-cache git && \
    git clone https://github.com/google-gemini/gemini-cli.git /tmp/gemini-cli && \
    cd /tmp/gemini-cli && \
    npm install && \
    npm install -g . && \
    rm -rf /tmp/gemini-cli

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/server /server

# Expose port 8080 to the outside world
EXPOSE 8080

# Command to run the executable. The Go application will call 'git' and 'gemini'.
CMD ["/server"]

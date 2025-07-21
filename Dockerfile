# Use the official Golang image to create a build artifact.
# This is the "builder" stage.
FROM golang:1.22-alpine AS builder

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
# Start a new, smaller stage from scratch
# This is the "runtime" stage.
FROM gcr.io/distroless/static-debian11

# Set the Current Working Directory inside the container
WORKDIR /

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/server /server

# Expose port 8080 to the outside world
EXPOSE 8080

# Command to run the executable
CMD ["/server"]

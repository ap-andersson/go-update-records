FROM golang:1.23

WORKDIR /app

# Copy mod files to working dir
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy empty config file
COPY config.yml ./

# Copy code to workdir
COPY *.go ./

# Compile code
RUN CGO_ENABLED=0 GOOS=linux go build -o /go-update-records

# Run program
CMD ["/go-update-records"]

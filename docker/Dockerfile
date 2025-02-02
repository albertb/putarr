# Copy sources to a builder image and build the binary for the Raspberry Pi.
FROM golang:1.23 AS builder
WORKDIR /go/src/github.com/albertb/putarr
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o putarr ./cmd/main.go

# Copy the binary to a new image.
FROM arm64v8/alpine:latest
COPY --from=builder /go/src/github.com/albertb/putarr/putarr /usr/local/bin/putarr

# Install dependencies.
RUN apk add tzdata
ENV TZ="America/New_York"

# Port, config volume and command.
EXPOSE 9091
VOLUME /config
ENTRYPOINT ["/usr/local/bin/putarr", "-config=/config/config.yaml"]
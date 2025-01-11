FROM golang:1.23 AS builder

WORKDIR /app

# Copy the source code into the container, and download the deps.
COPY . .
RUN go mod tidy

RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o putarr ./cmd/main.go

FROM arm64v8/alpine:latest

COPY --from=builder /app/putarr /usr/local/bin/putarr

# Install rclone
#RUN apk add --no-cache curl unzip bash && curl -s https://rclone.org/install.sh | bash
# TODO configure rclone with the same oauth token to mount put.io
# TODO run both putarr and rclone

EXPOSE 9091

VOLUME /config

ENTRYPOINT ["/usr/local/bin/putarr", "-config=/config/putarr.yaml"]

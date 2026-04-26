FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o fakegcp ./cmd/fakegcp

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/fakegcp /usr/local/bin/fakegcp
EXPOSE 8080
ENTRYPOINT ["fakegcp"]
CMD ["--port", "8080"]

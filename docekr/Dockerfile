FROM golang:1.21-alpine as builder
WORKDIR /app
COPY . .
RUN go build -o tailscale-go-proxy main.go

FROM alpine:latest
WORKDIR /app
RUN apk add --no-cache docker-cli
COPY --from=builder /app/tailscale-go-proxy .
COPY config.yaml .
EXPOSE 8081 1080 9091
CMD ["./tailscale-go-proxy"] 
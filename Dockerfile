# 第一阶段：构建 Go 可执行文件
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY  . .
RUN go mod download
RUN go build -o tailscale-go-proxy main.go

# 安装 tailscale 和 tailscaled
RUN go install tailscale.com/cmd/tailscale@latest
RUN go install tailscale.com/cmd/tailscaled@latest

# 最终运行镜像
FROM alpine:latest
WORKDIR /app
# 只安装运行所需的最小依赖
RUN apk add --no-cache iptables ip6tables docker-cli ca-certificates \
    && apk add --no-cache curl wget netcat-openbsd \
    && echo "hosts: files dns" > /etc/nsswitch.conf
COPY --from=builder /app/tailscale-go-proxy .
COPY --from=builder /go/bin/tailscale /usr/local/bin/tailscale
COPY --from=builder /go/bin/tailscaled /usr/local/bin/tailscaled
EXPOSE 1080 8081 1089
CMD ["./tailscale-go-proxy"]
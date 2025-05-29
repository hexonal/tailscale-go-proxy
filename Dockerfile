# 第一阶段：构建 Go 可执行文件
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY  . .
RUN go mod download
RUN go build -o tailscale-go-proxy main.go

# 安装 tailscale 和 tailscaled
RUN go install tailscale.com/cmd/tailscale@latest
RUN go install tailscale.com/cmd/tailscaled@latest

# 下载 gost 官方 release
FROM alpine:latest AS gostdl
WORKDIR /tmp
RUN wget -O gost.tar.gz https://github.com/go-gost/gost/releases/download/v3.0.0/gost_3.0.0_linux_amd64.tar.gz \
    && tar -xzf gost.tar.gz

# 最终运行镜像
FROM alpine:latest
WORKDIR /app
RUN apk add --no-cache iptables ip6tables
COPY --from=builder /app/tailscale-go-proxy .
COPY --from=builder /go/bin/tailscale /usr/local/bin/tailscale
COPY --from=builder /go/bin/tailscaled /usr/local/bin/tailscaled
COPY --from=gostdl /tmp/gost /usr/local/bin/gost
EXPOSE 1080 1089 8081
CMD ["tail", "-f", "/dev/null"] 
# 第一阶段：构建 Go 可执行文件
FROM golang:1.21-alpine as builder
WORKDIR /app
COPY ../.. .
RUN go build -o tailscale-go-proxy main.go

# 第二阶段：单独提取 docker-cli
FROM alpine:latest as dockercli
RUN apk add --no-cache docker-cli

# 第三阶段：最终运行镜像
FROM alpine:latest
WORKDIR /app
COPY --from=dockercli /usr/bin/docker /usr/bin/docker
COPY --from=dockercli /lib /lib
COPY --from=dockercli /lib64 /lib64
COPY --from=builder /app/tailscale-go-proxy .
COPY config.yaml .
EXPOSE 8081 1080 9091
CMD ["./tailscale-go-proxy"] 
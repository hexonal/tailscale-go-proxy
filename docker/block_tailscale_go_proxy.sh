#!/bin/bash

# 容器名称
CONTAINER_NAME="tailscale-go-proxy"
# 允许的本地代理端口
SOCKS5_PORT=1080

# 获取容器IP
CONTAINER_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' $CONTAINER_NAME)

if [ -z "$CONTAINER_IP" ]; then
  echo "未找到容器 $CONTAINER_NAME 的IP，请确认容器已启动。"
  exit 1
fi

echo "容器 $CONTAINER_NAME 的IP为: $CONTAINER_IP"

# 先清理旧规则（可选，防止重复添加）
sudo iptables -D DOCKER-USER -s $CONTAINER_IP -p tcp --dport $SOCKS5_PORT -j ACCEPT 2>/dev/null
sudo iptables -D DOCKER-USER -s $CONTAINER_IP -j DROP 2>/dev/null

# 允许容器访问本机1080端口（SOCKS5代理）
sudo iptables -I DOCKER-USER -s $CONTAINER_IP -d 127.0.0.1/32 -p tcp --dport $SOCKS5_PORT -j ACCEPT

# 禁止容器所有其它出站流量
sudo iptables -I DOCKER-USER -s $CONTAINER_IP -j DROP

echo "已完成规则设置：仅允许 $CONTAINER_NAME 通过本机 $SOCKS5_PORT 端口访问流量，其它全部禁止。" 
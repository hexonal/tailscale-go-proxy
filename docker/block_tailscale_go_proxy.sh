#!/bin/bash

# 容器名称
CONTAINER_NAME="tailscale-go-proxy"
# 允许的本地代理端口
SOCKS5_PORT=1080

# 需要放行的域名
ALLOWED_DOMAINS=("headscale.ipv4.name" "derp.ipv4.name")
# 需要放行的端口（可根据实际需求增减）
ALLOWED_PORTS=(443 80 50443 8080 3478)
ALLOWED_IPS=()

# 解析域名为IP
for domain in "${ALLOWED_DOMAINS[@]}"; do
  ips=$(dig +short $domain | grep -Eo '([0-9]{1,3}\.){3}[0-9]{1,3}')
  for ip in $ips; do
    ALLOWED_IPS+=($ip)
  done
done

# 获取容器IP
CONTAINER_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' $CONTAINER_NAME)

if [ -z "$CONTAINER_IP" ]; then
  echo "未找到容器 $CONTAINER_NAME 的IP，请确认容器已启动。"
  exit 1
fi

echo "容器 $CONTAINER_NAME 的IP为: $CONTAINER_IP"

# 清理旧规则
sudo iptables -D DOCKER-USER -s $CONTAINER_IP -p tcp --dport $SOCKS5_PORT -j ACCEPT 2>/dev/null
sudo iptables -D DOCKER-USER -s $CONTAINER_IP -d 100.64.0.0/10 -j ACCEPT 2>/dev/null
for ip in "${ALLOWED_IPS[@]}"; do
  for port in "${ALLOWED_PORTS[@]}"; do
    sudo iptables -D DOCKER-USER -s $CONTAINER_IP -d $ip -p tcp --dport $port -j ACCEPT 2>/dev/null
    sudo iptables -D DOCKER-USER -s $CONTAINER_IP -d $ip -p udp --dport $port -j ACCEPT 2>/dev/null
  done
  # 兼容所有协议所有端口的旧规则
  sudo iptables -D DOCKER-USER -s $CONTAINER_IP -d $ip -j ACCEPT 2>/dev/null
done
sudo iptables -D DOCKER-USER -s $CONTAINER_IP -j DROP 2>/dev/null

# 允许本机SOCKS5
sudo iptables -I DOCKER-USER -s $CONTAINER_IP -d 127.0.0.1/32 -p tcp --dport $SOCKS5_PORT -j ACCEPT

# 允许 Tailscale 内网
sudo iptables -I DOCKER-USER -s $CONTAINER_IP -d 100.64.0.0/10 -j ACCEPT

# 允许访问 headscale/derp 等域名的IP的所有必要端口
for ip in "${ALLOWED_IPS[@]}"; do
  for port in "${ALLOWED_PORTS[@]}"; do
    sudo iptables -I DOCKER-USER -s $CONTAINER_IP -d $ip -p tcp --dport $port -j ACCEPT
    sudo iptables -I DOCKER-USER -s $CONTAINER_IP -d $ip -p udp --dport $port -j ACCEPT
  done
  # 兼容所有协议所有端口（如需更严格可注释掉）
  sudo iptables -I DOCKER-USER -s $CONTAINER_IP -d $ip -j ACCEPT
done

# 禁止其它所有出站
sudo iptables -I DOCKER-USER -s $CONTAINER_IP -j DROP

echo "已完成规则设置：仅允许 $CONTAINER_NAME 通过本机 $SOCKS5_PORT、100.64.0.0/10 及 *.ipv4.name 域名的必要端口访问流量，其它全部禁止。"
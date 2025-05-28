package proxy

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"

	"tailscale-go-proxy/internal/cache"
)

// 启动 HTTP/HTTPS 代理服务
func StartHTTPProxy(addr string) error {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			handleHTTPS(w, r)
		} else {
			handleHTTP(w, r)
		}
	})
	log.Printf("HTTP/HTTPS 代理启动，监听地址: %s", addr)
	return http.ListenAndServe(addr, handler)
}

func handleHTTPS(w http.ResponseWriter, r *http.Request) {
	destConn, err := net.Dial("tcp", r.Host)
	if err != nil {
		http.Error(w, "无法连接目标服务器", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "不支持 Hijacker", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		return
	}
	go io.Copy(destConn, clientConn)
	go io.Copy(clientConn, destConn)
}

func handleHTTP(w http.ResponseWriter, r *http.Request) {
	// 构造新的请求
	req, err := http.NewRequest(r.Method, r.RequestURI, r.Body)
	if err != nil {
		http.Error(w, "请求构造失败", http.StatusBadRequest)
		return
	}
	req.Header = r.Header
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		http.Error(w, "转发失败: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// 工具函数：int 转 string
func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

// 从代理 URL 认证中提取 key（即密码部分）
func extractKeyFromURLUser(r *http.Request) string {
	if r.URL == nil || r.URL.User == nil {
		return ""
	}
	_, keySet := r.URL.User.Password()
	if !keySet {
		return ""
	}
	key, _ := r.URL.User.Password()
	return key
}

// 精确查找 key 对应节点
func findNodeByKey(nodeCache *cache.NodeCache, key string) *cache.Node {
	for _, n := range nodeCache.List() {
		if n.ID == key && n.Online && n.Device == "Android" {
			return n
		}
	}
	return nil
}

// 随机选一个在线 Android 节点
func pickRandomNode(nodeCache *cache.NodeCache) *cache.Node {
	nodes := nodeCache.List()
	var candidates []*cache.Node
	for _, n := range nodes {
		if n.Online && n.Device == "Android" {
			candidates = append(candidates, n)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	return candidates[rand.Intn(len(candidates))]
}

package cache

import (
	"sync"

	"github.com/gin-gonic/gin"
)

type Node struct {
	ID     string `json:"id"`
	IP     string `json:"ip"`
	Port   int    `json:"port"`
	Device string `json:"device"`
	Online bool   `json:"online"`
}

type NodeCache struct {
	mu    sync.RWMutex
	nodes map[string]*Node
}

func NewNodeCache() *NodeCache {
	return &NodeCache{
		nodes: make(map[string]*Node),
	}
}

func (c *NodeCache) Add(node *Node) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nodes[node.ID] = node
}

func (c *NodeCache) List() []*Node {
	c.mu.RLock()
	defer c.mu.RUnlock()
	list := make([]*Node, 0, len(c.nodes))
	for _, n := range c.nodes {
		list = append(list, n)
	}
	return list
}

func HandleGetNodes(c *gin.Context, cache *NodeCache) {
	nodes := cache.List()
	c.JSON(200, gin.H{"nodes": nodes})
}

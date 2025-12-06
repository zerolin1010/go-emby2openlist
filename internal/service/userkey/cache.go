package userkey

import (
	"sync"
	"time"
)

// CachedKey 缓存的 Key 信息
type CachedKey struct {
	Key       string
	ExpiredAt time.Time
}

// Cache 用户 Key 缓存
type Cache struct {
	data map[string]*CachedKey // key: userId 或 apiKey
	ttl  time.Duration
	mu   sync.RWMutex
}

// NewCache 创建缓存
func NewCache(ttl time.Duration) *Cache {
	c := &Cache{
		data: make(map[string]*CachedKey),
		ttl:  ttl,
	}

	// 启动定期清理过期缓存
	go c.cleanupLoop()

	return c
}

// Get 获取用户 Key (优先从缓存获取)
func (c *Cache) Get(userId string) (string, bool) {
	c.mu.RLock()
	cached, ok := c.data[userId]
	c.mu.RUnlock()

	if ok && time.Now().Before(cached.ExpiredAt) {
		return cached.Key, true
	}

	return "", false
}

// Set 设置用户 Key 缓存
func (c *Cache) Set(userId, apiKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[userId] = &CachedKey{
		Key:       apiKey,
		ExpiredAt: time.Now().Add(c.ttl),
	}
}

// Delete 删除缓存项
func (c *Cache) Delete(userId string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, userId)
}

// GetOrFetch 获取或使用原始 Key
func (c *Cache) GetOrFetch(userId, originalKey string) string {
	// 尝试从缓存获取
	if key, ok := c.Get(userId); ok {
		return key
	}

	// 对于简化方案，直接使用用户请求中的原始 key
	// 因为 Emby 的 key 验证已经在鉴权中间件完成
	c.Set(userId, originalKey)
	return originalKey
}

// cleanupLoop 定期清理过期缓存
func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(time.Minute * 5)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup 清理过期缓存
func (c *Cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for k, v := range c.data {
		if now.After(v.ExpiredAt) {
			delete(c.data, k)
		}
	}
}

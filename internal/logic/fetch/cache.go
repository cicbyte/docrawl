package fetch

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CacheManager HTML缓存管理器
type CacheManager struct {
	cacheDir string // 缓存目录
	metaFile string // 元数据文件
	mu       sync.RWMutex
	meta     *CacheMeta // 内存中的元数据
}

// CacheMeta 缓存元数据
type CacheMeta struct {
	Entries map[string]*CacheEntry `json:"entries"`
}

// CacheEntry 缓存条目
type CacheEntry struct {
	URL       string `json:"url"`
	File      string `json:"file"`       // 缓存文件名（MD5.html）
	FetchedAt string `json:"fetched_at"` // 抓取时间
	Status    int    `json:"status"`     // HTTP状态码（预留）
}

// NewCacheManager 创建缓存管理器
func NewCacheManager(outputDir string) (*CacheManager, error) {
	cacheDir := filepath.Join(outputDir, ".docrawl", "cache")
	metaFile := filepath.Join(outputDir, ".docrawl", "meta.json")

	cm := &CacheManager{
		cacheDir: cacheDir,
		metaFile: metaFile,
		meta: &CacheMeta{
			Entries: make(map[string]*CacheEntry),
		},
	}

	// 创建缓存目录
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("创建缓存目录失败: %w", err)
	}

	// 加载现有元数据
	cm.loadMeta()

	return cm, nil
}

// urlToHash 将URL转换为MD5哈希
func (cm *CacheManager) urlToHash(url string) string {
	hash := md5.Sum([]byte(url))
	return hex.EncodeToString(hash[:])
}

// loadMeta 加载元数据
func (cm *CacheManager) loadMeta() {
	data, err := os.ReadFile(cm.metaFile)
	if err != nil {
		return // 文件不存在，使用空元数据
	}

	var meta CacheMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return // 解析失败，使用空元数据
	}

	if meta.Entries == nil {
		meta.Entries = make(map[string]*CacheEntry)
	}

	cm.mu.Lock()
	cm.meta = &meta
	cm.mu.Unlock()
}

// saveMeta 保存元数据
func (cm *CacheManager) saveMeta() error {
	cm.mu.RLock()
	data, err := json.MarshalIndent(cm.meta, "", "  ")
	cm.mu.RUnlock()

	if err != nil {
		return err
	}

	return os.WriteFile(cm.metaFile, data, 0644)
}

// Get 从缓存获取HTML
func (cm *CacheManager) Get(url string) (string, bool) {
	hash := cm.urlToHash(url)
	cacheFile := filepath.Join(cm.cacheDir, hash+".html")

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return "", false
	}

	return string(data), true
}

// Set 保存HTML到缓存
func (cm *CacheManager) Set(url, html string) error {
	hash := cm.urlToHash(url)
	cacheFile := filepath.Join(cm.cacheDir, hash+".html")

	// 保存HTML文件
	if err := os.WriteFile(cacheFile, []byte(html), 0644); err != nil {
		return err
	}

	// 更新元数据
	cm.mu.Lock()
	cm.meta.Entries[url] = &CacheEntry{
		URL:       url,
		File:      hash + ".html",
		FetchedAt: time.Now().Format("2006-01-02 15:04:05"),
	}
	cm.mu.Unlock()

	// 保存元数据
	return cm.saveMeta()
}

// Has 检查缓存是否存在
func (cm *CacheManager) Has(url string) bool {
	cm.mu.RLock()
	_, exists := cm.meta.Entries[url]
	cm.mu.RUnlock()
	return exists
}

// GetCachePath 获取缓存文件路径
func (cm *CacheManager) GetCachePath(url string) string {
	hash := cm.urlToHash(url)
	return filepath.Join(cm.cacheDir, hash+".html")
}

// GetStats 获取缓存统计
func (cm *CacheManager) GetStats() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// 统计缓存文件大小
	var totalSize int64
	filepath.Walk(cm.cacheDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	return map[string]interface{}{
		"total_entries": len(cm.meta.Entries),
		"total_size":    totalSize,
		"cache_dir":     cm.cacheDir,
	}
}

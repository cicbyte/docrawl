package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/antchfx/htmlquery"
	"github.com/cicbyte/docrawl/internal/models"
	"github.com/cicbyte/docrawl/internal/processor"
	"github.com/cicbyte/docrawl/internal/renderer"
	"go.uber.org/zap"
	"golang.org/x/net/html"
)

// Config fetch命令配置
type Config struct {
	Input     string            // catalog.json路径
	Output    string            // 输出目录
	Workers   int               // 并发数
	Retries   int               // 重试次数
	Timeout   int               // 超时时间
	Delay     string            // 请求间等待时间，如 "20,50" 或 "20"
	AppConfig *models.AppConfig
}

// Progress 进度信息
type Progress struct {
	Total     int32
	Completed int32
	Failed    int32
	Current   string
}

// Processor fetch处理器
type Processor struct {
	config      *Config
	renderer    renderer.Renderer
	mdProcessor *processor.MarkdownProcessor
	cache       *CacheManager
	progress    *Progress
	logger      *zap.Logger
	mu          sync.Mutex
	// 层级目录缓存：level -> 目录路径
	levelPaths  map[int]string
	// 扁平化后的目录项（保留层级信息）
	flatItems   []*flatItem
	// OnProgress 进度回调（title, url, completed, total, failed）
	OnProgress func(title, url string, completed, total, failed int32, success bool)
}

// flatItem 扁平化的目录项（保留父级信息）
type flatItem struct {
	*CatalogItem
	Index      int          // 全局序号
	Parent     *CatalogItem // 直接父级
	ParentPath []pathPart   // 完整父级路径（从根到父）
}

// pathPart 路径部分
type pathPart struct {
	Title string
	Index int
}

// CatalogConfig 目录配置（从scan命令生成）
type CatalogConfig struct {
	Site      string         `json:"site"`
	URL       string         `json:"url"`
	Selectors Selectors      `json:"selectors"`
	Items     []*CatalogItem `json:"items"`
}

// Selectors 选择器配置
type Selectors struct {
	TOC         string `json:"toc"`
	Content     string `json:"content"`
	TOCType     string `json:"toc_type,omitempty"`    // 目录选择器类型: css/xpath
	ContentType string `json:"content_type,omitempty"` // 内容选择器类型: css/xpath
}

// CatalogItem 目录项
type CatalogItem struct {
	Title    string         `json:"title"`
	URL      string         `json:"url"`
	Children []*CatalogItem `json:"children,omitempty"`
}

// FetchResult 抓取结果
type FetchResult struct {
	Title      string
	URL        string
	OutputPath string
	WordCount  int
	Success    bool
	Error      error
}

// NewProcessor 创建fetch处理器
func NewProcessor(config *Config, logger *zap.Logger) (*Processor, error) {
	// 创建渲染器
	chromeRenderer := renderer.NewChromeDPRenderer(logger)

	// 创建Markdown处理器
	mdProcessor := processor.NewMarkdownProcessor(config.AppConfig.Crawler.IncludeMeta)

	// 创建缓存管理器
	cacheManager, err := NewCacheManager(config.Output)
	if err != nil {
		return nil, fmt.Errorf("创建缓存管理器失败: %w", err)
	}

	return &Processor{
		config:      config,
		renderer:    chromeRenderer,
		mdProcessor: mdProcessor,
		cache:       cacheManager,
		progress:    &Progress{},
		logger:      logger,
		levelPaths:  make(map[int]string),
	}, nil
}

// Execute 执行抓取
func (p *Processor) Execute(ctx context.Context) error {
	// 1. 加载catalog配置
	catalog, err := p.loadCatalog()
	if err != nil {
		return fmt.Errorf("加载目录配置失败: %w", err)
	}

	// 2. 展开目录项（保留父级信息）
	p.flatItems = p.flattenItemsWithParent(catalog.Items, nil, 0)
	p.progress.Total = int32(len(p.flatItems))

	p.logger.Info("开始抓取",
		zap.String("site", catalog.Site),
		zap.Int("total_pages", len(p.flatItems)),
		zap.Int("workers", p.config.Workers))

	// 3. 确保输出目录存在
	if err := os.MkdirAll(p.config.Output, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	// 4. 创建工作通道和结果通道
	itemChan := make(chan *flatItem, len(p.flatItems))
	resultChan := make(chan *FetchResult, len(p.flatItems))

	// 5. 填充工作通道
	for _, item := range p.flatItems {
		itemChan <- item
	}
	close(itemChan)

	// 6. 启动并发工作器
	var wg sync.WaitGroup
	for i := 0; i < p.config.Workers; i++ {
		wg.Add(1)
		go p.worker(ctx, itemChan, resultChan, catalog.Selectors, &wg)
	}

	// 7. 等待所有工作完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 8. 收集结果
	for result := range resultChan {
		if result.Success {
			atomic.AddInt32(&p.progress.Completed, 1)
			p.logger.Info("抓取成功",
				zap.String("url", result.URL),
				zap.String("output", result.OutputPath),
				zap.Int("word_count", result.WordCount))
		} else {
			atomic.AddInt32(&p.progress.Failed, 1)
			p.logger.Error("抓取失败",
				zap.String("url", result.URL),
				zap.Error(result.Error))
		}
		if p.OnProgress != nil {
			p.OnProgress(result.Title, result.URL, p.progress.Completed, p.progress.Total, p.progress.Failed, result.Success)
		}
	}

	// 9. 输出统计信息
	p.logger.Info("抓取完成",
		zap.Int32("total", p.progress.Total),
		zap.Int32("completed", p.progress.Completed),
		zap.Int32("failed", p.progress.Failed))

	return nil
}

// worker 工作协程
func (p *Processor) worker(ctx context.Context, itemChan <-chan *flatItem, resultChan chan<- *FetchResult, selectors Selectors, wg *sync.WaitGroup) {
	defer wg.Done()

	for item := range itemChan {
		select {
		case <-ctx.Done():
			return
		default:
		}
		result := p.fetchPage(ctx, item, selectors)
		resultChan <- result
		p.applyDelay()
	}
}

// applyDelay 根据配置应用请求间延迟
func (p *Processor) applyDelay() {
	delay := p.config.Delay
	if delay == "" {
		return
	}

	parts := strings.Split(delay, ",")
	if len(parts) == 2 {
		// 随机范围，如 "20,50"
		minMs, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		maxMs, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil || minMs < 0 || maxMs < minMs {
			p.logger.Warn("delay 参数格式错误，忽略", zap.String("delay", delay))
			return
		}
		ms := minMs + rand.Intn(maxMs-minMs+1)
		time.Sleep(time.Duration(ms) * time.Second)
	} else {
		// 固定延迟，如 "2"
		ms, err := strconv.Atoi(strings.TrimSpace(delay))
		if err != nil || ms < 0 {
			p.logger.Warn("delay 参数格式错误，忽略", zap.String("delay", delay))
			return
		}
		time.Sleep(time.Duration(ms) * time.Second)
	}
}

// fetchPage 抓取单个页面
func (p *Processor) fetchPage(ctx context.Context, item *flatItem, selectors Selectors) *FetchResult {
	result := &FetchResult{
		Title: item.Title,
		URL:   item.URL,
	}

	// 重试机制
	var lastErr error
	for retry := 0; retry <= p.config.Retries; retry++ {
		if retry > 0 {
			time.Sleep(time.Duration(p.config.AppConfig.Crawler.RetryDelay) * time.Second)
			p.logger.Debug("重试抓取",
				zap.String("url", item.URL),
				zap.Int("retry", retry))
		}

		err := p.doFetch(ctx, item, selectors, result)
		if err == nil {
			result.Success = true
			return result
		}
		lastErr = err
	}

	result.Success = false
	result.Error = lastErr
	return result
}

// doFetch 执行实际抓取
func (p *Processor) doFetch(ctx context.Context, item *flatItem, selectors Selectors, result *FetchResult) error {
	// 1. 生成输出文件名（支持层级目录）
	outputPath := p.generateOutputPath(item)
	result.OutputPath = outputPath

	// 2. 检查文件是否已存在，避免重复处理
	if p.isAlreadyProcessed(outputPath, item.URL) {
		p.logger.Info("文件已存在，跳过处理",
			zap.String("url", item.URL),
			zap.String("path", outputPath))
		result.WordCount = 0 // 已存在的文件不计入字数统计
		return nil
	}

	// 3. 尝试从缓存获取HTML
	var htmlContent string
	if cachedHTML, found := p.cache.Get(item.URL); found {
		p.logger.Debug("使用缓存HTML", zap.String("url", item.URL))
		htmlContent = cachedHTML
	} else {
		// 渲染页面
		renderReq := &renderer.RenderRequest{
			URL:     item.URL,
			Timeout: p.config.Timeout,
		}

		renderResp, err := p.renderer.Render(ctx, renderReq, &p.config.AppConfig.Crawler.ChromeDP)
		if err != nil {
			return fmt.Errorf("渲染页面失败: %w", err)
		}

		htmlContent = renderResp.HTML

		// 保存到缓存
		if err := p.cache.Set(item.URL, htmlContent); err != nil {
			p.logger.Warn("保存缓存失败", zap.Error(err))
		}
	}

	// 4. 提取内容
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return fmt.Errorf("解析HTML失败: %w", err)
	}

	content := p.extractContent(doc, selectors)
	if content == "" {
		return fmt.Errorf("未找到正文内容")
	}

	// 5. 转换为Markdown
	mdResult := p.mdProcessor.Process(content, item.URL)
	if !mdResult.Success {
		return fmt.Errorf("转换Markdown失败: %w", mdResult.Error)
	}

	result.WordCount = mdResult.WordCount

	// 6. 写入文件
	// 替换时间戳
	mdContent := strings.Replace(mdResult.Content, "<timestamp>", time.Now().Format("2006-01-02 15:04:05"), 1)
	if err := os.WriteFile(outputPath, []byte(mdContent), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	return nil
}

// isAlreadyProcessed 检查文件是否已处理过
func (p *Processor) isAlreadyProcessed(outputPath, url string) bool {
	// 检查文件是否存在
	info, err := os.Stat(outputPath)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil || info.IsDir() {
		return false
	}

	// 读取文件内容
	data, err := os.ReadFile(outputPath)
	if err != nil {
		return false
	}

	// 解析 YAML front matter 获取 source URL
	sourceURL := p.extractSourceFromMarkdown(data)
	if sourceURL == "" {
		return false
	}

	// 比较 URL
	return sourceURL == url
}

// extractSourceFromMarkdown 从 Markdown 文件中提取 source URL
func (p *Processor) extractSourceFromMarkdown(data []byte) string {
	content := string(data)

	// 检查是否有 YAML front matter
	if !strings.HasPrefix(content, "---\n") {
		return ""
	}

	// 查找 YAML 结束标记
	endIndex := strings.Index(content[4:], "\n---\n")
	if endIndex == -1 {
		return ""
	}

	yamlContent := content[4 : 4+endIndex]

	// 解析 source 字段
	lines := strings.Split(yamlContent, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "source:") {
			source := strings.TrimSpace(strings.TrimPrefix(line, "source:"))
			// 移除可能的引号
			source = strings.Trim(source, "\"'")
			return source
		}
	}

	return ""
}

// extractContent 提取正文内容（支持CSS和XPath选择器）
func (p *Processor) extractContent(doc *goquery.Document, selectors Selectors) string {
	selector := selectors.Content
	selectorType := selectors.ContentType

	// 默认使用CSS选择器
	if selectorType == "" {
		selectorType = "css"
	}

	if selector == "" || selector == "body" {
		selector = "body"
		selectorType = "css"
	}

	// XPath选择器
	if selectorType == "xpath" {
		content, err := p.extractByXPath(doc, selector)
		if err != nil {
			p.logger.Debug("XPath提取失败", zap.String("selector", selector), zap.Error(err))
			return ""
		}
		return content
	}

	// CSS选择器（默认）
	sel := doc.Find(selector)
	if sel.Length() == 0 {
		return ""
	}

	htmlStr, err := sel.Html()
	if err != nil {
		return ""
	}

	return htmlStr
}

// extractByXPath 使用XPath选择器提取内容
func (p *Processor) extractByXPath(doc *goquery.Document, xpath string) (string, error) {
	if len(doc.Nodes) == 0 {
		return "", fmt.Errorf("文档节点为空")
	}

	node, err := htmlquery.Query(doc.Nodes[0], xpath)
	if err != nil {
		return "", fmt.Errorf("XPath查询失败: %w", err)
	}
	if node == nil {
		return "", fmt.Errorf("XPath未找到匹配节点: %s", xpath)
	}

	var buf strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&buf, child); err != nil {
			return "", fmt.Errorf("渲染HTML失败: %w", err)
		}
	}

	return buf.String(), nil
}

// generateOutputPath 生成输出文件路径（按层级组织）
// 结构：
// - 有子内容：output/父标题/index.md（子内容在 父标题/子标题.md）
// - 无子内容：output/父标题/子标题.md
func (p *Processor) generateOutputPath(item *flatItem) string {
	safeTitle := p.sanitizeFilename(item.Title)
	hasChildren := len(item.Children) > 0

	// 构建完整的目录路径
	var pathParts []string
	pathParts = append(pathParts, p.config.Output)

	// 使用 ParentPath 构建父级目录
	for _, pp := range item.ParentPath {
		safeParentTitle := p.sanitizeFilename(pp.Title)
		pathParts = append(pathParts, safeParentTitle)
	}

	fullDir := filepath.Join(pathParts...)

	if hasChildren {
		// 有子内容：创建文件夹，内容保存为 index.md
		itemDir := filepath.Join(fullDir, safeTitle)
		if err := os.MkdirAll(itemDir, 0755); err != nil {
			p.logger.Warn("创建目录失败", zap.Error(err))
			return filepath.Join(fullDir, safeTitle+".md")
		}
		return filepath.Join(itemDir, "index.md")
	}

	// 无子内容：确保父级目录存在后，直接创建 .md 文件
	if len(item.ParentPath) > 0 {
		if err := os.MkdirAll(fullDir, 0755); err != nil {
			p.logger.Warn("创建父级目录失败", zap.Error(err))
		}
	}
	return filepath.Join(fullDir, safeTitle+".md")
}

// sanitizeFilename 清理文件名
func (p *Processor) sanitizeFilename(name string) string {
	// 移除或替换不安全的字符
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		"\n", "",
		"\r", "",
	)
	name = replacer.Replace(name)

	// 截断过长的文件名
	if len(name) > 100 {
		name = name[:100]
	}

	// 移除首尾空格
	name = strings.TrimSpace(name)

	// 如果为空，使用默认名称
	if name == "" {
		name = "untitled"
	}

	return name
}

// loadCatalog 加载目录配置
func (p *Processor) loadCatalog() (*CatalogConfig, error) {
	data, err := os.ReadFile(p.config.Input)
	if err != nil {
		return nil, err
	}

	var catalog CatalogConfig
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, err
	}

	return &catalog, nil
}

// flattenItemsWithParent 展开嵌套的目录项（保留父级信息和索引）
func (p *Processor) flattenItemsWithParent(items []*CatalogItem, parent *CatalogItem, startIndex int) []*flatItem {
	var result []*flatItem
	index := startIndex

	var flatten func([]*CatalogItem, *CatalogItem, []pathPart)
	flatten = func(list []*CatalogItem, parentItem *CatalogItem, parentPath []pathPart) {
		for _, item := range list {
			index++
			currentParentPath := make([]pathPart, len(parentPath))
			copy(currentParentPath, parentPath)

			result = append(result, &flatItem{
				CatalogItem: item,
				Index:       index,
				Parent:      parentItem,
				ParentPath:  currentParentPath,
			})

			if len(item.Children) > 0 {
				newParentPath := append(currentParentPath, pathPart{
					Title: item.Title,
					Index: index,
				})
				flatten(item.Children, item, newParentPath)
			}
		}
	}
	flatten(items, parent, nil)

	return result
}

// flattenItems 展开嵌套的目录项（兼容旧方法）
func (p *Processor) flattenItems(items []*CatalogItem) []*CatalogItem {
	var result []*CatalogItem

	var flatten func([]*CatalogItem)
	flatten = func(list []*CatalogItem) {
		for _, item := range list {
			result = append(result, item)
			if len(item.Children) > 0 {
				flatten(item.Children)
			}
		}
	}

	flatten(items)
	return result
}

// GetProgress 获取当前进度
func (p *Processor) GetProgress() *Progress {
	return p.progress
}

// validateConfig 验证配置
func validateConfig(config *Config) error {
	if config.Input == "" {
		return fmt.Errorf("输入文件不能为空")
	}
	if config.Output == "" {
		return fmt.Errorf("输出目录不能为空")
	}
	if config.Workers <= 0 {
		return fmt.Errorf("并发数必须大于0")
	}
	if config.Workers > 10 {
		return fmt.Errorf("并发数不能超过10")
	}
	return nil
}

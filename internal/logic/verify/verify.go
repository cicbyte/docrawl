package verify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/antchfx/htmlquery"
	"github.com/cicbyte/docrawl/internal/logic/fetch"
	"github.com/cicbyte/docrawl/internal/models"
	"github.com/cicbyte/docrawl/internal/processor"
	"github.com/cicbyte/docrawl/internal/renderer"
	"go.uber.org/zap"
	"golang.org/x/net/html"
)

// Config verify命令配置
type Config struct {
	URL       string // 要验证的URL
	Selector  string // 正文选择器
	Type      string // 选择器类型: css/xpath
	Catalog   string // catalog.json路径
	Output    string // 输出目录（用于缓存）
	Save      bool   // 是否保存到catalog
	AppConfig *models.AppConfig
}

// Result 验证结果
type Result struct {
	URL       string
	Selector  string
	Type      string
	Content   string
	WordCount int
	Success   bool
	Error     error
}

// CatalogConfig 目录配置（与fetch模块共享结构）
type CatalogConfig struct {
	Site        string         `json:"site"`
	URL         string         `json:"url"`
	Selectors   Selectors      `json:"selectors"`
	Items       []*CatalogItem `json:"items"`
	AIGenerated bool           `json:"ai_generated"`
	Confidence  float64        `json:"confidence"`
	ScannedAt   string         `json:"scanned_at"`
}

// Selectors 选择器配置
type Selectors struct {
	TOC         string `json:"toc"`
	Content     string `json:"content"`
	TOCType     string `json:"toc_type,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

// CatalogItem 目录项
type CatalogItem struct {
	Title    string         `json:"title"`
	URL      string         `json:"url"`
	Level    int            `json:"level"`
	Children []*CatalogItem `json:"children,omitempty"`
}

// Processor verify处理器
type Processor struct {
	config      *Config
	renderer    renderer.Renderer
	mdProcessor *processor.MarkdownProcessor
	cache       *fetch.CacheManager
	logger      *zap.Logger
}

// NewProcessor 创建verify处理器
func NewProcessor(config *Config, logger *zap.Logger) (*Processor, error) {
	// 创建渲染器
	chromeRenderer := renderer.NewChromeDPRenderer(logger)

	// 创建Markdown处理器
	mdProcessor := processor.NewMarkdownProcessor(config.AppConfig.Crawler.IncludeMeta)

	// 创建缓存管理器
	cacheManager, err := fetch.NewCacheManager(config.Output)
	if err != nil {
		return nil, fmt.Errorf("创建缓存管理器失败: %w", err)
	}

	return &Processor{
		config:      config,
		renderer:    chromeRenderer,
		mdProcessor: mdProcessor,
		cache:       cacheManager,
		logger:      logger,
	}, nil
}

// Execute 执行验证
func (p *Processor) Execute(ctx context.Context) (*Result, error) {
	result := &Result{
		URL:      p.config.URL,
		Selector: p.config.Selector,
		Type:     p.config.Type,
	}

	// 1. 尝试从缓存获取HTML
	var htmlContent string
	if cachedHTML, found := p.cache.Get(p.config.URL); found {
		p.logger.Debug("使用缓存HTML", zap.String("url", p.config.URL))
		htmlContent = cachedHTML
	} else {
		// 渲染页面
		renderReq := &renderer.RenderRequest{
			URL:     p.config.URL,
			Timeout: p.config.AppConfig.Crawler.PageTimeout,
		}

		renderResp, err := p.renderer.Render(ctx, renderReq, &p.config.AppConfig.Crawler.ChromeDP)
		if err != nil {
			result.Success = false
			result.Error = fmt.Errorf("渲染页面失败: %w", err)
			return result, nil
		}

		htmlContent = renderResp.HTML

		// 保存到缓存
		if err := p.cache.Set(p.config.URL, htmlContent); err != nil {
			p.logger.Warn("保存缓存失败", zap.Error(err))
		}
	}

	// 2. 解析HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		result.Success = false
		result.Error = fmt.Errorf("解析HTML失败: %w", err)
		return result, nil
	}

	// 3. 提取内容
	var content string
	if p.config.Type == "xpath" {
		content, err = p.extractByXPath(doc, p.config.Selector)
	} else {
		content, err = p.extractByCSS(doc, p.config.Selector), nil
	}

	if err != nil || content == "" {
		result.Success = false
		if err != nil {
			result.Error = err
		} else {
			result.Error = fmt.Errorf("未找到匹配的内容")
		}
		return result, nil
	}

	// 4. 转换为Markdown
	mdResult := p.mdProcessor.Process(content, p.config.URL)
	if !mdResult.Success {
		result.Success = false
		result.Error = fmt.Errorf("转换Markdown失败: %w", mdResult.Error)
		return result, nil
	}

	// 5. 设置结果（使用Markdown内容）
	// 替换时间戳占位符
	result.Content = strings.Replace(mdResult.Content, "<timestamp>", time.Now().Format("2006-01-02 15:04:05"), 1)
	result.WordCount = mdResult.WordCount
	result.Success = true

	return result, nil
}

// extractByCSS 使用CSS选择器提取内容
func (p *Processor) extractByCSS(doc *goquery.Document, selector string) string {
	if selector == "" || selector == "body" {
		selector = "body"
	}

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

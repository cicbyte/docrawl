package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	"github.com/cicbyte/docrawl/internal/models"
	"github.com/cicbyte/docrawl/internal/processor"
	"github.com/cicbyte/docrawl/internal/renderer"
	"github.com/cicbyte/docrawl/internal/utils"
	"go.uber.org/zap"
)

// Config scan命令配置
type Config struct {
	URL         string // 目标URL
	Output      string // 输出目录
	AIEnabled   bool   // 是否启用AI辅助
	Headless    bool   // 是否无头模式
	Timeout     int    // 超时时间
	ExpandTOC   bool   // 是否展开目录
	Interactive bool   // 交互模式
	AppConfig   *models.AppConfig
}

// Processor scan处理器
type Processor struct {
	config      *Config
	renderer    renderer.Renderer
	mdProcessor *processor.MarkdownProcessor
	aiClient    *utils.AIClient
	logger      *zap.Logger
}

// NewProcessor 创建scan处理器
func NewProcessor(config *Config, logger *zap.Logger) (*Processor, error) {
	// 创建渲染器
	chromeRenderer := renderer.NewChromeDPRenderer(logger)

	// 创建Markdown处理器
	mdProcessor := processor.NewMarkdownProcessor(config.AppConfig.Crawler.IncludeMeta)

	// 创建AI客户端
	var aiClient *utils.AIClient
	var err error
	if config.AIEnabled && config.AppConfig != nil {
		aiClient, err = utils.NewAIClient(&config.AppConfig.AI)
		if err != nil {
			logger.Warn("创建AI客户端失败，将使用降级模式", zap.Error(err))
		}
	}

	return &Processor{
		config:      config,
		renderer:    chromeRenderer,
		mdProcessor: mdProcessor,
		aiClient:    aiClient,
		logger:      logger,
	}, nil
}

// CatalogItem 目录项
type CatalogItem struct {
	Title    string         `json:"title"`
	URL      string         `json:"url"`
	Children []*CatalogItem `json:"children,omitempty"`
	level    int            `json:"-"` // 内部使用，不输出到 JSON
}

// CatalogConfig 目录配置
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
	TOC         string `json:"toc"`                    // 目录选择器
	Content     string `json:"content"`                // 内容选择器
	TOCType     string `json:"toc_type,omitempty"`     // 目录选择器类型: css/xpath
	ContentType string `json:"content_type,omitempty"` // 内容选择器类型: css/xpath
}

// Execute 执行扫描
func (p *Processor) Execute(ctx context.Context) (*CatalogConfig, error) {
	p.logger.Info("开始扫描文档站点", zap.String("url", p.config.URL))

	var htmlContent string

	// 自动展开目录模式
	if p.config.ExpandTOC {
		p.logger.Info("开始展开目录...")
		if err := p.expandTOC(ctx, &htmlContent); err != nil {
			return nil, fmt.Errorf("展开目录失败: %w", err)
		}
	} else {
		// 普通模式：使用渲染器获取页面HTML
		renderReq := &renderer.RenderRequest{
			URL:     p.config.URL,
			Timeout: p.config.Timeout,
		}

		renderResp, err := p.renderer.Render(ctx, renderReq, &p.config.AppConfig.Crawler.ChromeDP)
		if err != nil {
			return nil, fmt.Errorf("渲染页面失败: %w", err)
		}
		htmlContent = renderResp.HTML

		p.logger.Info("页面渲染完成",
			zap.String("url", renderResp.URL),
			zap.Int64("duration_ms", renderResp.Duration))
	}

	return p.processHTML(htmlContent)
}

// ExecuteInteractive 交互模式执行扫描
func (p *Processor) ExecuteInteractive(ctx context.Context, continueSignal <-chan struct{}) (*CatalogConfig, error) {
	p.logger.Info("开始交互模式扫描", zap.String("url", p.config.URL))

	var htmlContent string

	// 使用交互模式扫描
	if err := p.InteractiveScan(ctx, continueSignal, &htmlContent); err != nil {
		return nil, fmt.Errorf("交互扫描失败: %w", err)
	}

	return p.processHTML(htmlContent)
}

// processHTML 处理HTML并生成目录配置
func (p *Processor) processHTML(htmlContent string) (*CatalogConfig, error) {
	// 解析HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("解析HTML失败: %w", err)
	}

	// 检测目录区域
	tocSelector, tocItems, confidence, err := p.detectTOC(doc, htmlContent)
	if err != nil {
		return nil, fmt.Errorf("检测目录失败: %w", err)
	}

	// 检测内容区域
	contentSelector, err := p.detectContent(doc)
	if err != nil {
		return nil, fmt.Errorf("检测内容区域失败: %w", err)
	}

	// 构建目录配置
	parsedURL, _ := url.Parse(p.config.URL)
	site := parsedURL.Hostname()

	catalog := &CatalogConfig{
		Site: site,
		URL:  p.config.URL,
		Selectors: Selectors{
			TOC:     tocSelector,
			Content: contentSelector,
		},
		Items:       tocItems,
		AIGenerated: p.aiClient != nil,
		Confidence:  confidence,
		ScannedAt:   time.Now().Format(time.RFC3339),
	}

	// 保存目录配置
	if err := p.saveCatalog(catalog); err != nil {
		return nil, fmt.Errorf("保存目录配置失败: %w", err)
	}

	p.logger.Info("扫描完成",
		zap.String("toc_selector", tocSelector),
		zap.String("content_selector", contentSelector),
		zap.Int("items_count", len(tocItems)),
		zap.Float64("confidence", confidence))

	return catalog, nil
}

// detectTOC 检测目录区域
func (p *Processor) detectTOC(doc *goquery.Document, html string) (string, []*CatalogItem, float64, error) {
	// 常见的目录选择器
	candidateSelectors := []struct {
		selector string
		weight   float64
	}{
		{"nav.sidebar a", 0.95},
		{"aside nav a", 0.90},
		{".toc a", 0.85},
		{".sidebar a", 0.80},
		{".nav-links a", 0.75},
		{".menu a", 0.70},
		{".tree a", 0.70},
		{"nav ul a", 0.65},
		{".navigation a", 0.60},
		{"#toc a", 0.60},
	}

	var bestSelector string
	var bestItems []*CatalogItem
	var bestScore float64

	for _, candidate := range candidateSelectors {
		sel := doc.Find(candidate.selector)
		if sel.Length() < 3 {
			continue // 至少需要3个链接
		}

		// 检查链接是否指向同域名
		validLinks := 0
		baseURL, _ := url.Parse(p.config.URL)

		sel.Each(func(i int, s *goquery.Selection) {
			href, exists := s.Attr("href")
			if !exists || href == "" {
				return
			}

			linkURL, err := url.Parse(href)
			if err != nil {
				return
			}

			// 解析相对URL
			resolvedURL := baseURL.ResolveReference(linkURL)

			// 检查是否同域名
			if resolvedURL.Hostname() == baseURL.Hostname() {
				validLinks++
			}
		})

		score := float64(validLinks) * candidate.weight
		if score > bestScore {
			bestScore = score
			bestSelector = candidate.selector

			// 提取目录项
			bestItems = p.extractTOCItems(doc, candidate.selector)
		}
	}

	if bestSelector == "" {
		return "", nil, 0, fmt.Errorf("未找到合适的目录选择器")
	}

	confidence := bestScore / float64(len(bestItems)+1)
	if confidence > 1.0 {
		confidence = 1.0
	}

	return bestSelector, bestItems, confidence, nil
}

// extractTOCItems 提取目录项（构建嵌套 children 结构）
func (p *Processor) extractTOCItems(doc *goquery.Document, selector string) []*CatalogItem {
	var flatItems []*CatalogItem
	baseURL, _ := url.Parse(p.config.URL)

	doc.Find(selector).Each(func(i int, s *goquery.Selection) {
		title := strings.TrimSpace(s.Text())
		href, exists := s.Attr("href")
		if !exists || href == "" || title == "" {
			return
		}

		// 解析URL
		linkURL, err := url.Parse(href)
		if err != nil {
			return
		}

		// 解析相对URL
		resolvedURL := baseURL.ResolveReference(linkURL)

		// 检查是否同域名
		if resolvedURL.Hostname() != baseURL.Hostname() {
			return
		}

		// 计算层级（基于DOM层级）
		level := p.calculateLevel(s)

		flatItems = append(flatItems, &CatalogItem{
			Title: title,
			URL:   resolvedURL.String(),
			level: level, // 临时用于构建嵌套结构
		})
	})

	// 将扁平结构转换为嵌套结构
	return p.buildNestedItems(flatItems)
}

// buildNestedItems 将扁平的目录列表转换为嵌套结构
func (p *Processor) buildNestedItems(flatItems []*CatalogItem) []*CatalogItem {
	if len(flatItems) == 0 {
		return nil
	}

	var rootItems []*CatalogItem
	var stack []*CatalogItem // 栈：存储每层级的最后一个项

	for _, item := range flatItems {
		level := item.level // 使用临时字段

		if level == 1 {
			// 一级项：直接添加到根
			rootItems = append(rootItems, item)
			stack = []*CatalogItem{item}
		} else {
			// 子级项：找到父级并添加
			// 截断栈到当前层级-1
			if len(stack) >= level-1 {
				stack = stack[:level-1]
			}

			// 找到父级
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, item)

				// 更新栈
				if len(stack) >= level {
					stack[level-1] = item
				} else {
					stack = append(stack, item)
				}
			}
		}
		// 清除临时字段
		item.level = 0
	}

	return rootItems
}

// calculateLevel 计算目录项层级
// 层级从1开始，根据列表嵌套深度递增
func (p *Processor) calculateLevel(s *goquery.Selection) int {
	level := 0

	// 检查父元素中的列表嵌套层级
	parent := s.Parent()
	for parent.Length() > 0 {
		tag := strings.ToLower(parent.Nodes[0].Data)
		if tag == "ul" || tag == "ol" {
			level++
		}
		parent = parent.Parent()
	}

	// 确保最小层级为1
	if level == 0 {
		return 1
	}
	return level
}

// detectContent 检测内容区域
func (p *Processor) detectContent(doc *goquery.Document) (string, error) {
	// 常见的内容选择器
	contentSelectors := []struct {
		selector string
		weight   float64
	}{
		{"article.markdown-body", 0.95},
		{".markdown-body", 0.90},
		{"article.doc-content", 0.90},
		{"article", 0.85},
		{".content", 0.80},
		{".post-content", 0.80},
		{".article-content", 0.80},
		{".entry-content", 0.75},
		{"main", 0.70},
		{"[role='main']", 0.70},
		{".main", 0.65},
		{"#content", 0.60},
		{"#main", 0.60},
	}

	for _, candidate := range contentSelectors {
		if doc.Find(candidate.selector).Length() > 0 {
			return candidate.selector, nil
		}
	}

	return "body", nil
}

// saveCatalog 保存目录配置
func (p *Processor) saveCatalog(catalog *CatalogConfig) error {
	// 确保输出目录存在
	outputDir := p.config.Output
	if outputDir == "" {
		outputDir = "."
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	// 生成文件名
	siteName := strings.ReplaceAll(catalog.Site, ".", "-")
	siteName = strings.ReplaceAll(siteName, ":", "-")
	filename := fmt.Sprintf("catalog-%s.json", siteName)
	filePath := filepath.Join(outputDir, filename)

	// 序列化JSON
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return err
	}

	// 写入文件
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return err
	}

	p.logger.Info("目录配置已保存", zap.String("path", filePath))
	return nil
}

// AnalyzeWithAI 使用AI分析页面结构（可选）
func (p *Processor) AnalyzeWithAI(ctx context.Context, html string) (*AIAnalysisResult, error) {
	if p.aiClient == nil {
		return nil, fmt.Errorf("AI客户端未初始化")
	}

	systemPrompt := `你是一个网页结构分析专家。分析给定的HTML页面，识别文档目录区域和正文区域。

请严格按照以下JSON格式返回结果：
{
  "toc_selector": "CSS选择器，用于定位目录链接",
  "content_selector": "CSS选择器，用于定位正文内容",
  "confidence": 0.95,
  "reasoning": "选择这些选择器的原因"
}

分析要点：
1. 目录区域通常是包含多个链接的导航列表
2. 正文区域通常包含h1-h6标题、段落、代码块等
3. 选择器应该足够具体，避免误选
4. 置信度范围0-1，表示对选择的确定程度`

	userPrompt := fmt.Sprintf("请分析以下HTML页面结构：\n\n%s", html)

	response, err := p.aiClient.CallAISync(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}

	var result AIAnalysisResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("解析AI响应失败: %w", err)
	}

	return &result, nil
}

// AIAnalysisResult AI分析结果
type AIAnalysisResult struct {
	TOCSelector     string  `json:"toc_selector"`
	ContentSelector string  `json:"content_selector"`
	Confidence      float64 `json:"confidence"`
	Reasoning       string  `json:"reasoning"`
}

// InteractiveScan 交互模式扫描（带外部等待）
// 这个版本会保持浏览器打开，等待调用者通知继续
func (p *Processor) InteractiveScan(ctx context.Context, continueSignal <-chan struct{}, htmlContent *string) error {
	// 创建chromedp上下文（可见模式）
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx,
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("window-size", "1920,1080"),
		chromedp.Flag("start-maximized", true),
	)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	// 设置较长的超时时间
	timeout := 300 // 5分钟
	ctx, cancel := context.WithTimeout(taskCtx, time.Duration(timeout)*time.Second)
	defer cancel()

	// 导航到目标URL
	err := chromedp.Run(ctx,
		chromedp.Navigate(p.config.URL),
		chromedp.WaitReady("body"),
	)
	if err != nil {
		return fmt.Errorf("导航失败: %w", err)
	}

	// 等待用户确认信号
	select {
	case <-ctx.Done():
		return fmt.Errorf("操作超时")
	case <-continueSignal:
		// 用户确认继续
	}

	// 获取当前页面HTML
	err = chromedp.Run(ctx,
		chromedp.OuterHTML("html", htmlContent),
	)
	if err != nil {
		return fmt.Errorf("获取HTML失败: %w", err)
	}

	return nil
}

// expandTOC 展开折叠的目录项
// 使用chromedp点击所有可展开的目录项
func (p *Processor) expandTOC(ctx context.Context, htmlContent *string) error {
	// 创建chromedp上下文
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx,
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("headless", p.config.Headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("window-size", "1920,1080"),
	)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	// 设置超时
	timeout := p.config.Timeout
	if timeout <= 0 {
		timeout = 60
	}
	ctx, cancel := context.WithTimeout(taskCtx, time.Duration(timeout)*time.Second)
	defer cancel()

	// 执行展开和获取HTML的操作
	err := chromedp.Run(ctx,
		// 导航到目标URL
		chromedp.Navigate(p.config.URL),
		// 等待页面加载
		chromedp.WaitReady("body"),
		// 等待初始内容
		chromedp.Sleep(2*time.Second),

		// 展开所有可折叠的目录项
		chromedp.ActionFunc(func(ctx context.Context) error {
			return p.expandAllTOCItems(ctx)
		}),

		// 等待展开后的内容加载
		chromedp.Sleep(1*time.Second),

		// 获取展开后的HTML
		chromedp.OuterHTML("html", htmlContent),
	)

	return err
}

// expandAllTOCItems 展开所有可折叠的目录项
func (p *Processor) expandAllTOCItems(ctx context.Context) error {
	// 常见的可展开目录项选择器
	expandSelectors := []string{
		// 通用折叠按钮
		"button[aria-expanded='false']",
		".collapse-toggle:not(.open)",
		".expand-button:not(.expanded)",
		"[data-toggle='collapse']:not(.collapsed)",

		// VitePress/VuePress 风格
		".sidebar-group:not(.open) > .sidebar-heading",
		".nav-link[aria-expanded='false']",
		".vp-sidebar-group:not(.open) .caret",

		// Docusaurus 风格
		".menu__list-item--collapsed .menu__link--sublist",
		".theme-doc-sidebar-item-collapsed .menu__link--sublist",

		// GitBook 风格
		"[data-expanded='false']",

		// 通用箭头图标
		".arrow:not(.down)",
		".caret:not(.open)",
		".chevron:not(.rotated)",

		// DrissionPage 风格（根据用户提供的站点）
		"details:not([open]) > summary",
		".nav-item.collapsed > .nav-link",
		".folder:not(.open) > .folder-toggle",

		// Markdown 文档风格
		".toc-item:not(.expanded) .toc-toggle",
		".tree-item:not(.open) .tree-toggle",
	}

	maxIterations := 10 // 最多迭代10次，防止无限循环
	iteration := 0

	for iteration < maxIterations {
		clicked := false

		for _, selector := range expandSelectors {
			// 尝试点击该选择器的元素
			var clickedThisRound bool
			err := chromedp.Evaluate(fmt.Sprintf(`
				(function() {
					const els = document.querySelectorAll('%s');
					let clicked = false;
					for (const el of els) {
						// 检查元素是否可见且可点击
						const rect = el.getBoundingClientRect();
						if (rect.width > 0 && rect.height > 0) {
							try {
								el.click();
								clicked = true;
							} catch(e) {}
						}
					}
					return clicked;
				})()
			`, selector), &clickedThisRound).Do(ctx)

			if err == nil && clickedThisRound {
				clicked = true
				p.logger.Debug("展开目录项", zap.String("selector", selector))

				// 等待展开动画完成
				time.Sleep(300 * time.Millisecond)
			}
		}

		// 如果这一轮没有任何元素被点击，说明已经全部展开
		if !clicked {
			break
		}

		iteration++
	}

	p.logger.Info("目录展开完成", zap.Int("iterations", iteration))
	return nil
}

// smartExpandTOC 智能展开目录（基于DOM结构分析）
func (p *Processor) smartExpandTOC(ctx context.Context) error {
	// 使用JavaScript分析并展开所有折叠项
	script := `
	(function() {
		let expandedCount = 0;
		let iterations = 0;
		const maxIterations = 15;

		function expandAll() {
			if (iterations >= maxIterations) return;
			iterations++;

			let clicked = false;

			// 1. 查找并点击带有 aria-expanded="false" 的元素
			document.querySelectorAll('[aria-expanded="false"]').forEach(el => {
				if (el.offsetParent !== null) { // 可见元素
					el.click();
					clicked = true;
					expandedCount++;
				}
			});

			// 2. 查找并点击 details 元素
			document.querySelectorAll('details:not([open])').forEach(el => {
				if (el.offsetParent !== null) {
					el.setAttribute('open', '');
					clicked = true;
					expandedCount++;
				}
			});

			// 3. 查找并点击折叠的导航组
			document.querySelectorAll('.collapsed, .collapsed-folder, .folder-closed').forEach(el => {
				if (el.offsetParent !== null) {
					el.click();
					clicked = true;
					expandedCount++;
				}
			});

			// 4. 查找并点击带有 + 图标的项
			document.querySelectorAll('.icon-expand, .expand-icon, .toggle-icon').forEach(el => {
				if (el.offsetParent !== null && !el.classList.contains('expanded')) {
					el.click();
					clicked = true;
					expandedCount++;
				}
			});

			// 5. 查找 sidebar 中的可展开项
			document.querySelectorAll('.sidebar-group:not(.open), .nav-group:not(.open)').forEach(el => {
				const toggle = el.querySelector('.sidebar-heading, .nav-heading, .group-toggle');
				if (toggle && toggle.offsetParent !== null) {
					toggle.click();
					clicked = true;
					expandedCount++;
				}
			});

			// 如果有点击，继续检查
			if (clicked) {
				setTimeout(expandAll, 200);
			}
		}

		expandAll();
		return expandedCount;
	})()
	`

	var expandedCount int
	err := chromedp.Evaluate(script, &expandedCount).Do(ctx)
	if err != nil {
		return fmt.Errorf("执行展开脚本失败: %w", err)
	}

	p.logger.Info("智能展开完成", zap.Int("expanded_count", expandedCount))
	return nil
}

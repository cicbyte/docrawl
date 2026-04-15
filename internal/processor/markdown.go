package processor

import (
	"fmt"
	"regexp"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
)

// MarkdownProcessor Markdown处理器
type MarkdownProcessor struct {
	includeMeta bool
	converter   *md.Converter
}

// NewMarkdownProcessor 创建Markdown处理器
func NewMarkdownProcessor(includeMeta bool) *MarkdownProcessor {
	converter := md.NewConverter("", true, nil)

	return &MarkdownProcessor{
		includeMeta: includeMeta,
		converter:   converter,
	}
}

// ProcessResult 处理结果
type ProcessResult struct {
	Title     string
	Content   string
	WordCount int
	URL       string
	Success   bool
	Error     error
}

// Process 将HTML转换为Markdown
func (p *MarkdownProcessor) Process(htmlContent, url string) *ProcessResult {
	result := &ProcessResult{
		URL:     url,
		Success: true,
	}

	// 解析HTML提取标题
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err == nil {
		result.Title = p.extractTitle(doc)
	} else {
		result.Title = "未命名文档"
	}

	// 清理HTML：移除 class、id、style 等属性，保留语义
	cleanHTML := p.cleanHTML(htmlContent)

	// 使用 html-to-markdown 库转换
	content, err := p.converter.ConvertString(cleanHTML)
	if err != nil {
		result.Success = false
		result.Error = fmt.Errorf("转换Markdown失败: %w", err)
		return result
	}

	// 清理多余空行和残留的类名
	content = p.cleanMarkdown(content)

	// 添加YAML front matter
	var md strings.Builder
	if p.includeMeta {
		md.WriteString("---\n")
		md.WriteString(fmt.Sprintf("title: %s\n", result.Title))
		md.WriteString(fmt.Sprintf("source: %s\n", url))
		md.WriteString("fetched_at: <timestamp>\n")
		md.WriteString(fmt.Sprintf("word_count: %d\n", p.countWords(content)))
		md.WriteString("---\n\n")
	}
	md.WriteString(content)

	result.Content = md.String()
	result.WordCount = p.countWords(content)

	return result
}

// cleanHTML 清理HTML，移除不必要的属性
func (p *MarkdownProcessor) cleanHTML(htmlContent string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return htmlContent
	}

	// 移除所有元素的 class、id、style 属性
	doc.Find("*").Each(func(i int, s *goquery.Selection) {
		s.RemoveAttr("class")
		s.RemoveAttr("id")
		s.RemoveAttr("style")
		s.RemoveAttr("data-*")
	})

	// 移除不需要的元素
	doc.Find("script, style, nav, header, footer, aside, .ad, .advertisement").Remove()

	html, err := doc.Find("body").Html()
	if err != nil {
		return htmlContent
	}
	return html
}

// extractTitle 提取标题
func (p *MarkdownProcessor) extractTitle(doc *goquery.Document) string {
	// 尝试从h1标签获取标题
	if title := doc.Find("h1").First().Text(); title != "" {
		return strings.TrimSpace(title)
	}

	// 尝试从title标签获取
	if title := doc.Find("title").First().Text(); title != "" {
		return strings.TrimSpace(title)
	}

	return "未命名文档"
}

// cleanMarkdown 清理Markdown格式
func (p *MarkdownProcessor) cleanMarkdown(content string) string {
	// 移除"本页总览"等无意义标识
	content = regexp.MustCompile(`(?i)本页总览[\s]*`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?i)页面总览[\s]*`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?i)目录[\s]*\n`).ReplaceAllString(content, "\n")

	// 移除残留的类名模式（如 codeBlockLines_e6Vv）
	// 要求：小写开头 + 下划线 + 小写字母 + 数字（哈希后缀特征）
	// 这样可以匹配 codeBlockLines_e6Vv 但不会匹配 get_tab
	classNamePattern := regexp.MustCompile(`\b[a-z]+_[a-z]+[a-z0-9]*_[a-zA-Z0-9]+\b`)
	content = classNamePattern.ReplaceAllString(content, "")

	// 移除代码块中不确定的语言标识（```后跟无意义字符）
	// 将 ```text 或 ```unknown 等改为 ```
	codeBlockLang := regexp.MustCompile("```\\s*[a-zA-Z0-9_]+\\s*\n")
	content = codeBlockLang.ReplaceAllStringFunc(content, func(match string) string {
		lang := regexp.MustCompile("```\\s*([a-zA-Z0-9_]+)").FindStringSubmatch(match)
		if len(lang) > 1 {
			// 只保留常见语言
			commonLangs := map[string]bool{
				"go": true, "python": true, "javascript": true, "js": true,
				"typescript": true, "ts": true, "java": true, "c": true,
				"cpp": true, "csharp": true, "cs": true, "ruby": true,
				"rust": true, "php": true, "swift": true, "kotlin": true,
				"scala": true, "html": true, "css": true, "sql": true,
				"bash": true, "shell": true, "sh": true, "json": true,
				"yaml": true, "xml": true, "markdown": true, "md": true,
			}
			langLower := strings.ToLower(lang[1])
			if !commonLangs[langLower] {
				return "```\n"
			}
		}
		return match
	})

	// 移除多余的空行（超过2个连续空行）
	re := regexp.MustCompile(`\n{3,}`)
	content = re.ReplaceAllString(content, "\n\n")

	// 移除行首行尾空白
	lines := strings.Split(content, "\n")
	var cleaned []string
	for _, line := range lines {
		cleaned = append(cleaned, strings.TrimRight(line, " \t"))
	}
	content = strings.Join(cleaned, "\n")

	return strings.TrimSpace(content) + "\n"
}

// countWords 计算字数
func (p *MarkdownProcessor) countWords(text string) int {
	// 移除Markdown标记
	text = regexp.MustCompile(`[#*_\[\]()>` + "`" + `~-]`).ReplaceAllString(text, "")
	text = regexp.MustCompile(`\n+`).ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	count := 0
	inWord := false

	for _, r := range text {
		if r == ' ' || r == '\t' || r == '\n' {
			inWord = false
			continue
		}

		// 中文字符单独计数
		if r >= 0x4e00 && r <= 0x9fff {
			count++
			inWord = false
		} else if !inWord {
			count++
			inWord = true
		}
	}

	return count
}

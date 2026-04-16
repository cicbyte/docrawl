# docrawl

文档抓取工具 - 将任意文档网站转换为结构化 Markdown

docrawl 是一个 CLI 工具，可以将任意文档网站（VitePress、Docusaurus、GitBook 等）自动转换为结构化的 Markdown 文件。通过 ChromeDP 渲染页面、自动识别目录结构、智能提取正文内容，并支持 AI 辅助分析提升抓取质量。

## 功能特性

- **智能扫描**: 自动检测文档站点的目录结构和正文区域
- **AI 分析**: 使用 AI 分析页面截图，识别最佳目录选择器
- **选择器验证**: verify 命令支持验证和调试 CSS/XPath 选择器
- **并发抓取**: 多工作协程并发抓取，内置失败重试
- **实时进度**: 抓取过程中显示进度条、当前页面和成功/失败状态
- **请求延迟**: 支持固定或随机请求间隔，避免对目标站点造成过大压力
- **优雅退出**: 支持 Ctrl+C 中断，安全停止抓取
- **Markdown 转换**: 完整支持代码块、表格、链接、图片等格式
- **YAML Front Matter**: 自动生成包含元数据的 Markdown 文件

## 快速开始

### 前置要求

- Go 1.24+
- Windows 10/11（当前仅在 Windows 平台做开发测试，Linux 和 macOS 未做可用性验证）
- Chrome/Chromium 浏览器（用于页面渲染）
- AI API 密钥（可选，用于 AI 辅助分析，默认使用智谱 AI GLM-4-Flash）

### 安装

```bash
# go install（注意：版本信息为默认值 dev）
go install github.com/cicbyte/docrawl@latest

# 从源码编译
git clone https://github.com/cicbyte/docrawl.git
cd docrawl
go mod download
go build -o docrawl.exe  # Windows
go build -o docrawl      # Linux/macOS
```

完整构建（含前端资源 + UPX 压缩）：

```bash
python build.py
```

## 使用说明

### 1. 扫描文档站点 (scan)

扫描文档站点，自动识别目录结构，生成 `catalog.json`。

```bash
docrawl scan https://docs.example.com
docrawl scan https://docs.example.com -o ./my-catalog   # 指定输出目录
docrawl scan https://docs.example.com --expand           # 展开折叠目录
docrawl scan https://docs.example.com --interactive      # 交互模式（显示浏览器）
```

| 参数 | 简写 | 默认值 | 说明 |
|------|------|--------|------|
| `--output` | `-o` | `.` | 输出目录 |
| `--ai` | | `true` | 启用 AI 辅助分析 |
| `--headless` | | `true` | 无头模式运行浏览器 |
| `--timeout` | `-t` | `60` | 页面加载超时时间(秒) |
| `--expand` | `-e` | `false` | 自动展开折叠的目录项 |
| `--interactive` | `-i` | `false` | 交互模式：显示浏览器，等待用户操作 |

### 2. 抓取内容 (fetch)

根据 `catalog.json` 并发抓取页面，输出层级化 Markdown 文件。

```bash
docrawl fetch -i catalog.json -o ./output
docrawl fetch -i catalog.json -o ./output -w 5 -r 3     # 自定义并发和重试
docrawl fetch -i catalog.json -o ./output -d 1,3        # 请求间随机等待1~3秒
docrawl fetch -i catalog.json -o ./output -d 2          # 请求间固定等待2秒
```

| 参数 | 简写 | 默认值 | 说明 |
|------|------|--------|------|
| `--input` | `-i` | (必需) | catalog.json 配置文件路径 |
| `--output` | `-o` | `./output` | 输出目录 |
| `--workers` | `-w` | `3` | 并发数 (1-10) |
| `--retries` | `-r` | `3` | 重试次数 |
| `--timeout` | `-t` | `60` | 页面加载超时时间(秒) |
| `--delay` | `-d` | | 请求间等待时间(秒)，如 `1,3` 随机或 `2` 固定 |

### 3. 验证选择器 (verify)

验证 CSS/XPath 选择器，预览提取效果，可保存修改到 catalog.json。

```bash
docrawl verify -u https://example.com/page -s "article.content" -t css
docrawl verify -i catalog.json -u https://example.com/page --save
```

### 输出格式

每个抓取的页面生成一个 Markdown 文件，包含 YAML Front Matter：

```markdown
---
title: 页面标题
source: https://docs.example.com/page
fetched_at: 2026-04-15T10:00:00Z
word_count: 1500
---

# 页面标题

正文内容...
```

## 配置

配置文件路径：`~/.cicbyte/docrawl/config/config.yaml`（首次运行自动创建）

```yaml
ai:
  provider: openai
  api_key: ""
  base_url: https://open.bigmodel.cn/api/paas/v4/
  model: GLM-4-Flash-250414
  max_tokens: 2048
  temperature: 0.8
  timeout: 30

crawler:
  concurrency: 3
  request_timeout: 30
  page_timeout: 60
  max_retries: 3
  retry_delay: 1
  save_raw_html: false
  include_meta: true
  chromedp:
    enabled: true
    headless: true
    wait_timeout: 30
    wait_delay: 1

log:
  level: info
  max_size: 10
  max_backups: 30
  max_age: 30
  compress: true
```

## 技术栈

- **CLI 框架**: [Cobra](https://github.com/spf13/cobra)
- **Web 自动化**: [chromedp](https://github.com/chromedp/chromedp)
- **HTML 解析**: [goquery](https://github.com/PuerkitoBio/goquery)
- **ORM**: [GORM](https://gorm.io/)
- **日志**: [uber-zap](https://github.com/uber-go/zap)
- **配置**: [go.yaml.in/yaml/v3](https://github.com/go-yaml/yaml)

## 贡献指南

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 打开一个 Pull Request

## 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情

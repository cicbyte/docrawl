# docrawl

文档抓取工具 - 将任意文档网站转换为结构化 Markdown

## 描述

docrawl 是一个强大的文档抓取工具，可以将任意文档网站转换为结构化的 Markdown 格式。它支持：

- 自动识别文档目录结构
- AI 辅助分析页面布局
- 智能提取正文内容
- 高并发抓取与重试机制
- 转换为格式规范的 Markdown

## 功能特性

- **智能扫描**: 自动检测文档站点的目录结构和正文区域
- **AI 分析**: 使用 AI 分析页面截图，识别最佳目录选择器
- **并发抓取**: 支持多工作协程并发抓取，提高效率
- **重试机制**: 内置失败重试，确保抓取完整性
- **Markdown 转换**: 完整支持代码块、表格、链接、图片等格式
- **YAML Front Matter**: 自动生成包含元数据的 Markdown 文件

## 快速开始

### 前置要求

- Go 1.24+
- Chrome/Chromium 浏览器（用于页面渲染）
- AI API 密钥（可选，用于 AI 辅助分析）

### 安装步骤

```bash
# 克隆仓库
git clone https://github.com/cicbyte/docrawl.git
cd docrawl

# 安装依赖
go mod download

# 编译
go build -o docrawl.exe  # Windows
go build -o docrawl      # Linux/macOS
```

## 使用说明

### 基本用法

#### 1. 扫描文档站点

```bash
# 扫描文档站点，生成目录配置
docrawl scan https://docs.example.com

# 使用 AI 辅助分析
docrawl scan https://docs.example.com --ai

# 指定输出文件
docrawl scan https://docs.example.com -o catalog.json
```

#### 2. 抓取内容

```bash
# 根据目录配置抓取内容
docrawl fetch -i catalog.json -o ./output

# 指定并发数和重试次数
docrawl fetch -i catalog.json -o ./output -w 5 -r 3

# 保存原始 HTML
docrawl fetch -i catalog.json -o ./output --save-html
```

### 命令参数

#### scan 命令

| 参数 | 简写 | 默认值 | 说明 |
|------|------|--------|------|
| `--output` | `-o` | `catalog.json` | 输出配置文件路径 |
| `--ai` | `-a` | `false` | 启用 AI 辅助分析 |
| `--headless` | | `true` | 无头模式运行浏览器 |
| `--timeout` | `-t` | `60` | 页面加载超时时间(秒) |

#### fetch 命令

| 参数 | 简写 | 默认值 | 说明 |
|------|------|--------|------|
| `--input` | `-i` | (必需) | catalog.json 配置文件路径 |
| `--output` | `-o` | `./output` | 输出目录 |
| `--workers` | `-w` | `3` | 并发数 (1-10) |
| `--retries` | `-r` | `3` | 重试次数 |
| `--save-html` | | `false` | 保存原始 HTML |
| `--timeout` | `-t` | `60` | 页面加载超时时间(秒) |

### 输出格式

每个抓取的页面会生成一个 Markdown 文件，格式如下：

```markdown
---
title: 页面标题
source: https://docs.example.com/page
fetched_at: 2026-03-13T10:00:00Z
word_count: 1500
---

# 页面标题

正文内容...
```

### 配置选项

配置文件位于 `~/.ciclebyte/docrawl/config/config.yaml`

```yaml
ai:
  provider: zhipu
  api_key: your-api-key
  model: glm-4-flash
  base_url: https://open.bigmodel.cn/api/paas/v4

crawler:
  concurrency: 3
  request_timeout: 30
  page_timeout: 60
  max_retries: 3
  retry_delay: 2
  save_raw_html: false
  include_meta: true
  chromedp:
    enabled: true
    headless: true
    wait_timeout: 30
    wait_delay: 1

database:
  type: sqlite
  path: ~/.ciclebyte/docrawl/db/app.db

log:
  level: info
  max_size: 100
  max_backups: 3
  max_age: 7
```

## 项目结构

```
docrawl/
├── main.go              # 应用入口
├── cmd/                 # CLI 命令定义
│   ├── root.go         # 根命令
│   ├── scan/           # scan 命令
│   └── fetch/          # fetch 命令
├── internal/            # 内部包
│   ├── common/         # 全局变量
│   ├── log/            # 日志模块
│   ├── models/         # 数据模型
│   ├── utils/          # 工具函数
│   ├── renderer/       # ChromeDP 渲染器
│   ├── processor/      # Markdown 处理器
│   └── logic/          # 业务逻辑层
│       ├── scan/       # scan 逻辑
│       └── fetch/      # fetch 逻辑
└── resources/          # 资源文件
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
2. 创建你的特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交你的更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 打开一个 Pull Request

## 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情

## 版本历史

- 0.1.0
  - 初始版本
  - 实现 scan 和 fetch 命令
  - 支持 AI 辅助分析
  - 完整的 Markdown 转换功能

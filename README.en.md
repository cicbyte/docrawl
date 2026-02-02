简体中文 | [English](README.md)

# docrawl

> A CLI tool that converts any documentation site (VitePress, Docusaurus, GitBook, etc.) into structured Markdown.

![Release](https://img.shields.io/github/v/release/cicbyte/docrawl?style=flat) ![Go Report Card](https://goreportcard.com/badge/github.com/cicbyte/docrawl) ![License](https://img.shields.io/github/license/cicbyte/docrawl) ![Last Commit](https://img.shields.io/github/last-commit/cicbyte/docrawl)

docrawl renders pages via ChromeDP, automatically identifies navigation structures, intelligently extracts content, and supports AI-assisted analysis for improved crawl quality.

## Features

- **Smart Scanning** — Automatically detects site navigation and content regions, with support for expanding collapsed items
- **AI Analysis** — Uses AI to analyze page screenshots and identify optimal content selectors
- **Concurrent Fetching** — Multi-worker concurrent crawling with real-time progress bar
- **Request Delay** — Fixed or randomized delays between requests to avoid overwhelming target sites
- **Markdown Conversion** — Preserves code blocks, tables, links, and images; auto-generates YAML front matter
- **Selector Verification** — `verify` command for debugging CSS/XPath selectors
- **Graceful Shutdown** — Safe interruption via Ctrl+C without losing already-fetched content

## Installation

```bash
go install github.com/cicbyte/docrawl@latest
```

> Note: `go install` does not inject version info — the version will display as `dev`. For full version info, download binaries from [Releases](https://github.com/cicbyte/docrawl/releases).

**Build from source:**

```bash
git clone https://github.com/cicbyte/docrawl.git
cd docrawl
go build -o docrawl.exe
```

### Requirements

- Go 1.24+
- Windows 10/11 (currently only tested on Windows; Linux and macOS compatibility not verified)
- Chrome/Chromium browser (for page rendering)
- AI API key (optional, defaults to Zhipu AI GLM-4-Flash)

## Quick Start

```bash
# 1. Scan a documentation site, generates catalog.json
docrawl scan https://docs.golang.org

# 2. Fetch content using catalog.json
docrawl fetch -i catalog.json -o ./output

# 3. Check results
ls ./output
```

## Usage

### `docrawl scan` — Scan Documentation Site

Automatically identifies navigation structure and generates `catalog.json` for fetch.

```bash
docrawl scan https://docs.example.com
docrawl scan https://docs.example.com -o ./my-catalog   # Custom output directory
docrawl scan https://docs.example.com --expand           # Expand collapsed nav items
docrawl scan https://docs.example.com --interactive      # Interactive mode (visible browser)
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output` | `-o` | `.` | Output directory |
| `--ai` | | `true` | Enable AI-assisted analysis |
| `--headless` | | `true` | Run browser in headless mode |
| `--timeout` | `-t` | `60` | Page load timeout (seconds) |
| `--expand` | `-e` | `false` | Auto-expand collapsed navigation items |
| `--interactive` | `-i` | `false` | Interactive mode: show browser and wait for user input |

### `docrawl fetch` — Fetch Content

Concurrently fetches pages based on `catalog.json` and outputs hierarchical Markdown files.

```bash
docrawl fetch -i catalog.json -o ./output
docrawl fetch -i catalog.json -o ./output -w 5 -r 3     # Custom concurrency and retries
docrawl fetch -i catalog.json -o ./output -d 1,3        # Random delay of 1~3 seconds between requests
docrawl fetch -i catalog.json -o ./output -d 2          # Fixed delay of 2 seconds between requests
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--input` | `-i` | (required) | Path to catalog.json |
| `--output` | `-o` | `./output` | Output directory |
| `--workers` | `-w` | `3` | Concurrency (1-10) |
| `--retries` | `-r` | `3` | Retry count |
| `--timeout` | `-t` | `60` | Page load timeout (seconds) |
| `--delay` | `-d` | | Delay between requests (seconds), e.g. `1,3` for random or `2` for fixed |

### `docrawl verify` — Verify Selectors

Verify CSS/XPath selectors, preview extraction results, and save changes to catalog.json.

```bash
docrawl verify -u https://example.com/page -s "article.content" -t css
docrawl verify -i catalog.json -u https://example.com/page --save
```

### Output Format

Each fetched page generates a Markdown file with YAML front matter:

```markdown
---
title: Page Title
source: https://docs.example.com/page
fetched_at: 2026-04-15T10:00:00Z
word_count: 1500
---

# Page Title

Content goes here...
```

## Configuration

Config file path: `~/.cicbyte/docrawl/config/config.yaml` (auto-created on first run)

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

## Tech Stack

- **CLI Framework**: [Cobra](https://github.com/spf13/cobra)
- **Browser Automation**: [chromedp](https://github.com/chromedp/chromedp)
- **HTML Parsing**: [goquery](https://github.com/PuerkitoBio/goquery)
- **ORM**: [GORM](https://gorm.io/)
- **Logging**: [uber-zap](https://github.com/uber-go/zap)
- **Configuration**: [go.yaml.in/yaml/v3](https://github.com/go-yaml/yaml)

## Contributing

1. Fork this repository
2. Create a feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## License

[MIT](LICENSE)

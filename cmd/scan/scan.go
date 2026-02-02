package scan

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cicbyte/docrawl/internal/common"
	"github.com/cicbyte/docrawl/internal/log"
	"github.com/cicbyte/docrawl/internal/logic/scan"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	// 命令参数
	scanURL         string
	scanOutput      string
	scanAI          bool
	scanHeadless    bool
	scanTimeout     int
	scanExpand      bool
	scanInteractive bool
)

// GetCommand 获取scan命令
func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan <URL>",
		Short: "扫描文档站点，生成目录配置",
		Long: `扫描文档网站，自动识别目录结构和内容区域，生成catalog.json配置文件。

使用方式:
  docrawl scan https://docs.example.com
  docrawl scan https://docs.example.com -o ./output
  docrawl scan https://docs.example.com --expand
  docrawl scan https://docs.example.com --interactive
  docrawl scan https://docs.example.com --no-ai

参数说明:
  URL: 要扫描的文档站点首页URL
  --expand: 自动展开折叠的目录项
  --interactive: 交互模式，显示浏览器让用户手动操作

输出:
  生成catalog.json配置文件，包含：
  - 目录选择器
  - 内容选择器
  - 目录项列表（标题、URL、层级）`,
		Args: cobra.ExactArgs(1),
		Run:  runScan,
	}

	// 添加参数
	cmd.Flags().StringVarP(&scanOutput, "output", "o", ".", "输出目录")
	cmd.Flags().BoolVar(&scanAI, "ai", true, "启用AI辅助分析")
	cmd.Flags().BoolVar(&scanHeadless, "headless", true, "无头模式运行")
	cmd.Flags().IntVarP(&scanTimeout, "timeout", "t", 60, "页面加载超时时间(秒)")
	cmd.Flags().BoolVarP(&scanExpand, "expand", "e", false, "自动展开折叠的目录项")
	cmd.Flags().BoolVarP(&scanInteractive, "interactive", "i", false, "交互模式：显示浏览器，等待用户操作后按Enter继续")

	return cmd
}

func runScan(cmd *cobra.Command, args []string) {
	// 获取URL参数
	scanURL = args[0]

	// 验证参数
	if err := validateParams(); err != nil {
		fmt.Printf("❌ 参数验证失败: %v\n", err)
		os.Exit(1)
	}

	// 创建处理器配置
	config := &scan.Config{
		URL:         scanURL,
		Output:      scanOutput,
		AIEnabled:   scanAI,
		Headless:    scanHeadless,
		Timeout:     scanTimeout,
		ExpandTOC:   scanExpand,
		Interactive: scanInteractive,
		AppConfig:   common.AppConfigModel,
	}

	// 交互模式强制关闭无头模式
	if scanInteractive {
		config.Headless = false
	}

	// 创建处理器
	logger := log.GetLogger()
	processor, err := scan.NewProcessor(config, logger)
	if err != nil {
		fmt.Printf("❌ 创建处理器失败: %v\n", err)
		os.Exit(1)
	}

	// 执行扫描
	fmt.Printf("🔍 开始扫描: %s\n", scanURL)

	if scanInteractive {
		// 交互模式
		fmt.Println("🖥️  交互模式：正在打开浏览器...")
		fmt.Println("📝 请在浏览器中手动展开目录或进行其他操作")
		fmt.Println("⏎  完成后请在终端按 Enter 键继续...")

		// 创建继续信号通道
		continueChan := make(chan struct{})

		// 启动goroutine等待用户输入
		go func() {
			reader := bufio.NewReader(os.Stdin)
			reader.ReadString('\n')
			close(continueChan)
		}()

		catalog, err := processor.ExecuteInteractive(context.Background(), continueChan)
		if err != nil {
			fmt.Printf("❌ 扫描失败: %v\n", err)
			logger.Error("扫描失败", zap.Error(err))
			os.Exit(1)
		}

		printResult(catalog)
	} else {
		// 非交互模式
		if scanExpand {
			fmt.Println("📂 正在展开目录...")
		}
		fmt.Println("📡 正在加载页面...")

		catalog, err := processor.Execute(context.Background())
		if err != nil {
			fmt.Printf("❌ 扫描失败: %v\n", err)
			logger.Error("扫描失败", zap.Error(err))
			os.Exit(1)
		}

		printResult(catalog)
	}
}

// printResult 打印扫描结果
func printResult(catalog *scan.CatalogConfig) {
	// 输出结果
	fmt.Println()
	fmt.Println("✅ 扫描完成!")
	fmt.Println()
	fmt.Printf("📄 站点: %s\n", catalog.Site)
	fmt.Printf("📋 目录选择器: %s\n", catalog.Selectors.TOC)
	fmt.Printf("📝 内容选择器: %s\n", catalog.Selectors.Content)
	fmt.Printf("📊 目录项数量: %d\n", len(catalog.Items))
	fmt.Printf("🎯 置信度: %.0f%%\n", catalog.Confidence*100)
	fmt.Printf("🤖 AI辅助: %v\n", catalog.AIGenerated)

	if len(catalog.Items) > 0 {
		fmt.Println()
		fmt.Println("📑 目录预览:")
		printItems(catalog.Items, 0, 10)
	}

	fmt.Println()
	fmt.Printf("💾 配置已保存到: catalog-%s.json\n", catalog.Site)
}

// validateParams 验证参数
func validateParams() error {
	if scanURL == "" {
		return fmt.Errorf("URL不能为空")
	}

	// 检查URL格式
	if len(scanURL) < 7 || (scanURL[:7] != "http://" && scanURL[:8] != "https://") {
		return fmt.Errorf("URL必须以http://或https://开头")
	}

	if scanTimeout <= 0 {
		return fmt.Errorf("超时时间必须大于0")
	}

	return nil
}

// printItems 递归打印目录项
func printItems(items []*scan.CatalogItem, depth int, maxItems int) int {
	count := 0
	for _, item := range items {
		if count >= maxItems {
			return count
		}
		indent := strings.Repeat("  ", depth)
		fmt.Printf("  %s• %s\n", indent, item.Title)
		count++
		if len(item.Children) > 0 {
			count += printItems(item.Children, depth+1, maxItems-count)
		}
	}
	return count
}

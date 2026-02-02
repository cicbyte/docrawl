package fetch

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/cicbyte/docrawl/internal/common"
	"github.com/cicbyte/docrawl/internal/log"
	"github.com/cicbyte/docrawl/internal/logic/fetch"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	// 命令参数
	fetchInput   string
	fetchOutput  string
	fetchWorkers int
	fetchRetries int
	fetchTimeout int
	fetchDelay   string
)

// GetCommand 获取fetch命令
func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "抓取文档内容，输出Markdown文件",
		Long: `根据catalog.json配置抓取文档内容，转换为Markdown格式输出。

使用方式:
  docrawl fetch -i catalog.json -o ./output
  docrawl fetch -i catalog.json -o ./output -w 5

参数说明:
  -i, --input: catalog.json配置文件路径
  -o, --output: 输出目录

输出:
  按层级组织的Markdown文件:
  - 父标题/
    - index.md
    - 子标题.md

每个Markdown文件包含:
  - YAML front matter（标题、来源、时间、字数）
  - 正文内容

HTML缓存:
  原始HTML自动缓存到 output/.docrawl/cache/ 目录，重新运行时复用缓存`,
		Run: runFetch,
	}

	// 添加参数
	cmd.Flags().StringVarP(&fetchInput, "input", "i", "", "catalog.json配置文件路径（必需）")
	cmd.Flags().StringVarP(&fetchOutput, "output", "o", "./output", "输出目录")
	cmd.Flags().IntVarP(&fetchWorkers, "workers", "w", 3, "并发数（1-10）")
	cmd.Flags().IntVarP(&fetchRetries, "retries", "r", 3, "重试次数")
	cmd.Flags().IntVarP(&fetchTimeout, "timeout", "t", 60, "页面加载超时时间(秒)")
	cmd.Flags().StringVarP(&fetchDelay, "delay", "d", "", "请求间随机等待时间(秒)，如 1,3 表示1~3秒随机，仅输入2表示固定等待2秒")

	// 标记必需参数
	cmd.MarkFlagRequired("input")

	return cmd
}

func runFetch(cmd *cobra.Command, args []string) {
	// 验证参数
	if err := validateParams(); err != nil {
		fmt.Printf("参数验证失败: %v\n", err)
		os.Exit(1)
	}

	// 创建处理器配置
	config := &fetch.Config{
		Input:     fetchInput,
		Output:    fetchOutput,
		Workers:   fetchWorkers,
		Retries:   fetchRetries,
		Timeout:   fetchTimeout,
		Delay:     fetchDelay,
		AppConfig: common.AppConfigModel,
	}

	// 创建处理器
	logger := log.GetLogger()
	processor, err := fetch.NewProcessor(config, logger)
	if err != nil {
		fmt.Printf("创建处理器失败: %v\n", err)
		os.Exit(1)
	}

	// 设置进度回调
	processor.OnProgress = func(title, url string, completed, total, failed int32, success bool) {
		percent := float64(completed) / float64(total) * 100
		bar := renderBar(int(percent), 20)
		status := "OK"
		if !success {
			status = "FAIL"
		}

		// 截断过长的标题
		if len(title) > 40 {
			title = title[:37] + "..."
		}

		fmt.Printf("\r%s [%5.1f%%] %d/%d | %-4s | %s",
			bar, percent, completed, total, status, title)
	}

	fmt.Printf("输入: %s\n", fetchInput)
	fmt.Printf("输出: %s\n", fetchOutput)
	fmt.Printf("并发: %d\n", fetchWorkers)

	// 监听 Ctrl+C 优雅退出
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n收到中断信号，正在停止...")
		cancel()
	}()

	if err := processor.Execute(ctx); err != nil {
		fmt.Printf("\n抓取失败: %v\n", err)
		logger.Error("抓取失败", zap.Error(err))
		os.Exit(1)
	}

	// 输出统计
	progress := processor.GetProgress()
	fmt.Println()
	fmt.Println("抓取完成!")
	fmt.Printf("总计: %d 页  成功: %d 页", progress.Total, progress.Completed)
	if progress.Failed > 0 {
		fmt.Printf("  失败: %d 页", progress.Failed)
	}
	fmt.Println()
	fmt.Printf("输出目录: %s\n", fetchOutput)
}

// renderBar 渲染进度条
func renderBar(percent, width int) string {
	filled := width * percent / 100
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
}

// validateParams 验证参数
func validateParams() error {
	if fetchInput == "" {
		return fmt.Errorf("输入文件不能为空")
	}

	// 检查输入文件是否存在
	if _, err := os.Stat(fetchInput); os.IsNotExist(err) {
		return fmt.Errorf("输入文件不存在: %s", fetchInput)
	}

	if fetchWorkers <= 0 || fetchWorkers > 10 {
		return fmt.Errorf("并发数必须在1-10之间")
	}

	if fetchTimeout <= 0 {
		return fmt.Errorf("超时时间必须大于0")
	}

	return nil
}

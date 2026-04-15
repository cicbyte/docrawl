/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/cicbyte/docrawl/cmd/fetch"
	"github.com/cicbyte/docrawl/cmd/scan"
	"github.com/cicbyte/docrawl/cmd/verify"
	"github.com/cicbyte/docrawl/internal/common"
	"github.com/cicbyte/docrawl/internal/log"
	"github.com/cicbyte/docrawl/internal/utils"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// 构建时通过 -ldflags 注入
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
	GitBranch = "unknown"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "docrawl",
	Short: "文档抓取工具 - 将任意文档网站转换为结构化Markdown",
	Long: `docrawl 是一个强大的文档抓取工具，可以将任意文档网站转换为结构化的Markdown格式。

核心功能:
  - 自动识别文档目录结构
  - AI辅助分析页面布局
  - 智能提取正文内容
  - 转换为Markdown格式

使用方式:
  docrawl scan https://docs.example.com           # 扫描文档站点
  docrawl fetch -i catalog.json -o ./output       # 抓取内容

更多信息请访问: https://github.com/cicbyte/docrawl`,
}

func init() {
	rootCmd.SetVersionTemplate("{{.Version}}\n")
	rootCmd.Version = fmt.Sprintf("docrawl %s (%s) %s/%s", Version, BuildTime, GitBranch, GitCommit[:7])
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// 初始化应用目录
	if err := utils.InitAppDirs(); err != nil {
		fmt.Printf("初始化目录失败: %v\n", err)
		os.Exit(1)
	}
	// 加载配置(会自动创建默认配置)
	common.AppConfigModel = utils.ConfigInstance.LoadConfig()
	// 初始化日志
	if err := log.Init(utils.ConfigInstance.GetLogPath()); err != nil {
		fmt.Printf("日志初始化失败: %v\n", err)
		os.Exit(1)
	}

	// 初始化数据库连接
	if _, err := utils.GetGormDB(); err != nil {
		log.Error("数据库连接失败",
			zap.String("operation", "db init"),
			zap.Error(err))
		os.Exit(1)
	}
	log.Info("数据库连接成功")

	// 注册子命令
	rootCmd.AddCommand(scan.GetCommand())
	rootCmd.AddCommand(fetch.GetCommand())
	rootCmd.AddCommand(verify.GetCommand())
}
package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cicbyte/docrawl/internal/common"
	"github.com/cicbyte/docrawl/internal/log"
	logicVerify "github.com/cicbyte/docrawl/internal/logic/verify"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	verifyURL      string
	verifySelector string
	verifyType     string
	verifyCatalog  string
	verifyOutput   string
	verifySave     bool
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "验证并编辑正文选择器",
		Long: `验证选择器能否正确提取页面内容，预览并保存到catalog.json。

使用方式:
  # 验证catalog.json中的选择器
  docrawl verify -i catalog.json -u https://example.com/page

  # 验证自定义CSS选择器
  docrawl verify -u https://example.com/page -s "article.content" -t css

  # 验证XPath选择器
  docrawl verify -u https://example.com/page -s "//article[@class='content']" -t xpath

  # 验证并保存修改到catalog.json
  docrawl verify -i catalog.json -u https://example.com/page -s "article.new" --save`,
		Run: runVerify,
	}

	cmd.Flags().StringVarP(&verifyURL, "url", "u", "", "要验证的URL（必填）")
	cmd.Flags().StringVarP(&verifySelector, "selector", "s", "", "正文选择器（覆盖catalog配置）")
	cmd.Flags().StringVarP(&verifyType, "type", "t", "", "选择器类型(css/xpath)，默认css")
	cmd.Flags().StringVarP(&verifyCatalog, "input", "i", "", "catalog.json路径")
	cmd.Flags().StringVarP(&verifyOutput, "output", "o", "./output", "输出目录（用于缓存）")
	cmd.Flags().BoolVarP(&verifySave, "save", "S", false, "保存选择器到catalog.json")

	cmd.MarkFlagRequired("url")

	return cmd
}

func runVerify(cmd *cobra.Command, args []string) {
	logger := log.GetLogger()

	// 验证参数
	if verifyURL == "" {
		logger.Error("URL不能为空")
		os.Exit(1)
	}

	// 确定选择器类型
	selectorType := verifyType
	if selectorType == "" {
		selectorType = "css"
	}
	if selectorType != "css" && selectorType != "xpath" {
		logger.Error("选择器类型必须是 css 或 xpath", zap.String("type", selectorType))
		os.Exit(1)
	}

	// 加载catalog配置（如果提供）
	var catalog *logicVerify.CatalogConfig
	if verifyCatalog != "" {
		var err error
		catalog, err = loadCatalog(verifyCatalog)
		if err != nil {
			logger.Error("加载catalog失败", zap.Error(err))
			os.Exit(1)
		}
	}

	// 确定选择器
	selector := verifySelector
	if selector == "" && catalog != nil {
		selector = catalog.Selectors.Content
		selectorType = catalog.Selectors.ContentType
		if selectorType == "" {
			selectorType = "css"
		}
	}
	if selector == "" {
		logger.Error("未指定选择器，请使用 -s 参数或在 catalog.json 中配置")
		os.Exit(1)
	}

	// 创建处理器
	processor, err := logicVerify.NewProcessor(&logicVerify.Config{
		URL:       verifyURL,
		Selector:  selector,
		Type:      selectorType,
		Catalog:   verifyCatalog,
		Output:    verifyOutput,
		Save:      verifySave,
		AppConfig: common.AppConfigModel,
	}, logger)
	if err != nil {
		logger.Error("创建处理器失败", zap.Error(err))
		os.Exit(1)
	}

	// 执行验证
	result, err := processor.Execute(context.Background())
	if err != nil {
		logger.Error("验证失败", zap.Error(err))
		os.Exit(1)
	}

	// 显示结果
	displayResult(result)

	// 如果需要保存且验证成功
	if verifySave && result.Success {
		if catalog == nil {
			logger.Error("未提供catalog文件，无法保存")
			os.Exit(1)
		}

		// 更新catalog
		catalog.Selectors.Content = selector
		catalog.Selectors.ContentType = selectorType

		if err := saveCatalog(verifyCatalog, catalog); err != nil {
			logger.Error("保存catalog失败", zap.Error(err))
			os.Exit(1)
		}
		fmt.Println("\n📝 已更新 catalog.json")
	}
}

func loadCatalog(path string) (*logicVerify.CatalogConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var catalog logicVerify.CatalogConfig
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, err
	}

	return &catalog, nil
}

func saveCatalog(path string, catalog *logicVerify.CatalogConfig) error {
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func displayResult(result *logicVerify.Result) {
	fmt.Println()
	fmt.Println("🔍 验证选择器:", result.Selector)
	fmt.Println("📄 页面:", result.URL)
	fmt.Println("🎯 类型:", result.Type)
	fmt.Println()

	if !result.Success {
		fmt.Println("❌ 验证失败:", result.Error)
		return
	}

	fmt.Println("📖 内容预览 (前50行):")
	fmt.Println("────────────────────────────────────────")

	// 显示前50行
	lines := strings.Split(result.Content, "\n")
	maxLines := 50
	if len(lines) < maxLines {
		maxLines = len(lines)
	}
	for i := 0; i < maxLines; i++ {
		fmt.Println(lines[i])
	}
	if len(lines) > 50 {
		fmt.Printf("... (还有 %d 行)\n", len(lines)-50)
	}

	fmt.Println("────────────────────────────────────────")
	fmt.Println()
	fmt.Printf("📊 统计: 字数 %d, 字符 %d\n", result.WordCount, len(result.Content))
	fmt.Println()
	fmt.Println("✅ 选择器验证成功!")

	if verifySave {
		fmt.Println("💾 选择器已标记保存")
	}
}

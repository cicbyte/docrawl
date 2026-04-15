package renderer

import (
	"context"

	"github.com/cicbyte/docrawl/internal/models"
)

// RenderRequest 渲染请求
type RenderRequest struct {
	URL     string                 // 目标URL
	Timeout int                    // 超时时间（秒）
	Options map[string]interface{} // 额外选项
}

// RenderResponse 渲染响应
type RenderResponse struct {
	HTML        string // 渲染后的HTML
	URL         string // 最终URL（可能有重定向）
	ContentType string // 内容类型
	Duration    int64  // 渲染耗时（毫秒）
	Screenshot  []byte // 截图数据（可选）
}

// Renderer 渲染器接口
type Renderer interface {
	// Type 返回渲染器类型
	Type() RendererType

	// Name 返回渲染器名称
	Name() string

	// IsAvailable 检查渲染器是否可用
	IsAvailable(ctx context.Context, config *models.ChromeDPConfig) bool

	// Render 执行渲染
	Render(ctx context.Context, req *RenderRequest, config *models.ChromeDPConfig) (*RenderResponse, error)

	// ValidateConfig 验证配置
	ValidateConfig(config *models.ChromeDPConfig) error

	// GetDefaultOptions 获取默认选项
	GetDefaultOptions() map[string]interface{}
}

// RendererType 渲染器类型
type RendererType string

const (
	// RendererTypeChromeDP ChromeDP渲染器
	RendererTypeChromeDP RendererType = "chromedp"
	// RendererTypeNone 无渲染（直接HTTP请求）
	RendererTypeNone RendererType = "none"
)

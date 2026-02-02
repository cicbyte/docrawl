package renderer

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/cicbyte/docrawl/internal/models"
	"go.uber.org/zap"
)

// ChromeDPRenderer Chrome DevTools Protocol渲染器
type ChromeDPRenderer struct {
	allocatorContext context.Context
	allocatorCancel  context.CancelFunc
	logger           *zap.Logger
}

// NewChromeDPRenderer 创建ChromeDP渲染器
func NewChromeDPRenderer(logger *zap.Logger) Renderer {
	return &ChromeDPRenderer{
		logger: logger,
	}
}

// Type 返回渲染器类型
func (r *ChromeDPRenderer) Type() RendererType {
	return RendererTypeChromeDP
}

// Name 返回渲染器名称
func (r *ChromeDPRenderer) Name() string {
	return "Chrome DevTools Protocol渲染器"
}

// IsAvailable 检查渲染器是否可用
func (r *ChromeDPRenderer) IsAvailable(ctx context.Context, config *models.ChromeDPConfig) bool {
	if config == nil {
		return false
	}

	// 检查ChromeDP是否启用
	if !config.Enabled {
		return false
	}

	// 如果配置了远程调试URL，检查连接
	if config.RemoteDebugURL != "" {
		return r.checkConnection(ctx, config.RemoteDebugURL)
	}

	// 如果配置了Chrome路径，检查文件是否存在
	if config.ChromePath != "" {
		if _, err := os.Stat(config.ChromePath); err != nil {
			return false
		}
	}

	return true
}

// checkConnection 检查Chrome调试连接是否可用
func (r *ChromeDPRenderer) checkConnection(ctx context.Context, url string) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url + "/json/version")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Render 执行渲染
func (r *ChromeDPRenderer) Render(ctx context.Context, req *RenderRequest, config *models.ChromeDPConfig) (*RenderResponse, error) {
	startTime := time.Now()

	if !r.IsAvailable(ctx, config) {
		return nil, fmt.Errorf("ChromeDP渲染器不可用")
	}

	// 设置超时
	timeout := config.WaitTimeout
	if timeout <= 0 {
		timeout = 30
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// 获取调试URL
	debugURL := r.getDebugURL(config)

	// 创建allocator上下文
	var allocatorCtx context.Context
	var allocatorCancel context.CancelFunc
	var keepAllocator bool

	// 判断是否尝试复用现有浏览器
	shouldReuse := config.ReuseBrowser && debugURL != ""

	if shouldReuse {
		// 尝试复用现有浏览器
		var err error
		allocatorCtx, allocatorCancel, err = r.tryReuseBrowser(ctx, debugURL, config)
		if err != nil {
			return nil, fmt.Errorf("ChromeDP浏览器复用失败: %w", err)
		}
		keepAllocator = config.KeepBrowser
	} else {
		// 创建新的浏览器实例
		allocatorCtx, allocatorCancel = chromedp.NewExecAllocator(ctx,
			r.buildExecAllocatorOptions(config)...,
		)
		keepAllocator = config.KeepBrowser
	}

	// 只有在非保持模式下才defer cancel
	if !keepAllocator {
		defer allocatorCancel()
	}

	// 创建任务上下文
	taskCtx, cancelTask := chromedp.NewContext(allocatorCtx)
	if !keepAllocator {
		defer cancelTask()
	}

	// 获取窗口尺寸
	windowWidth := config.WindowWidth
	windowHeight := config.WindowHeight
	if windowWidth <= 0 {
		windowWidth = 1920
	}
	if windowHeight <= 0 {
		windowHeight = 1080
	}

	var htmlContent string

	// 执行HTML渲染任务
	err := chromedp.Run(taskCtx,
		r.buildRenderingActions(req.URL, windowWidth, windowHeight, config, &htmlContent),
	)

	if err != nil {
		return nil, fmt.Errorf("ChromeDP渲染失败: %w", err)
	}

	duration := time.Since(startTime).Milliseconds()

	return &RenderResponse{
		HTML:        htmlContent,
		URL:         req.URL,
		Duration:    duration,
		ContentType: "text/html",
	}, nil
}

// getDebugURL 获取调试URL
func (r *ChromeDPRenderer) getDebugURL(config *models.ChromeDPConfig) string {
	// 优先使用配置的远程调试URL
	if config.RemoteDebugURL != "" {
		return config.RemoteDebugURL
	}

	// 否则使用本地调试端口
	debugPort := config.DebugPort
	if debugPort <= 0 {
		debugPort = 9222
	}
	return fmt.Sprintf("http://localhost:%d", debugPort)
}

// tryReuseBrowser 尝试复用浏览器
func (r *ChromeDPRenderer) tryReuseBrowser(ctx context.Context, debugURL string, config *models.ChromeDPConfig) (context.Context, context.CancelFunc, error) {
	// 先检查连接是否可用
	if r.checkConnection(ctx, debugURL) {
		// 连接成功，创建远程allocator
		allocatorCtx, allocatorCancel := chromedp.NewRemoteAllocator(ctx, debugURL)
		return allocatorCtx, allocatorCancel, nil
	}

	// 调试端口不可用，尝试启动Chrome
	if err := r.startChromeWithDebugPort(config); err != nil {
		return nil, nil, fmt.Errorf("启动Chrome失败: %w", err)
	}

	// Chrome启动后再次检查连接
	maxWait := 5 * time.Second
	startTime := time.Now()

	for time.Since(startTime) < maxWait {
		if r.checkConnection(ctx, debugURL) {
			allocatorCtx, allocatorCancel := chromedp.NewRemoteAllocator(ctx, debugURL)
			return allocatorCtx, allocatorCancel, nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return nil, nil, fmt.Errorf("Chrome启动后仍无法连接到调试端口: %s", debugURL)
}

// findChromePath 查找Chrome浏览器路径
func (r *ChromeDPRenderer) findChromePath(config *models.ChromeDPConfig) string {
	// 优先使用配置的Chrome路径
	if config.ChromePath != "" {
		if _, err := os.Stat(config.ChromePath); err == nil {
			return config.ChromePath
		}
	}

	// 根据操作系统自动查找Chrome
	var possiblePaths []string

	switch runtime.GOOS {
	case "windows":
		possiblePaths = []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			filepath.Join(os.Getenv("LOCALAPPDATA"), `Google\Chrome\Application\chrome.exe`),
		}
	case "darwin":
		possiblePaths = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		}
	case "linux":
		possiblePaths = []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium-browser",
			"/usr/bin/chromium",
			"/snap/bin/chromium",
		}
	}

	for _, path := range possiblePaths {
		if runtime.GOOS == "windows" {
			path = os.ExpandEnv(path)
		}
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// startChromeWithDebugPort 启动Chrome浏览器并开启调试端口
func (r *ChromeDPRenderer) startChromeWithDebugPort(config *models.ChromeDPConfig) error {
	chromePath := r.findChromePath(config)
	if chromePath == "" {
		return fmt.Errorf("未找到Chrome浏览器")
	}

	debugPort := config.DebugPort
	if debugPort <= 0 {
		debugPort = 9222
	}

	// 检查端口是否已被占用
	debugURL := fmt.Sprintf("http://localhost:%d", debugPort)
	if r.checkConnection(context.Background(), debugURL) {
		return nil
	}

	// 创建专用的用户数据目录
	userDataDir := ""
	if config.UserDataDir != "" {
		userDataDir = fmt.Sprintf("%s_debug_%d", config.UserDataDir, debugPort)
	} else {
		userDataDir = filepath.Join(os.TempDir(), fmt.Sprintf("chrome_debug_%d", debugPort))
	}

	// 确保用户数据目录存在
	if err := os.MkdirAll(userDataDir, 0755); err != nil {
		return fmt.Errorf("创建用户数据目录失败: %w", err)
	}

	// 构建启动参数
	args := []string{
		"--remote-debugging-port=" + fmt.Sprintf("%d", debugPort),
		"--remote-debugging-address=0.0.0.0",
		"--user-data-dir=" + userDataDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
		"--disable-dev-shm-usage",
		"--disable-extensions",
		"--disable-plugins",
		"--disable-background-networking",
		"--disable-sync",
		"--disable-default-apps",
	}

	if config.NoSandbox {
		args = append(args, "--no-sandbox")
	}
	if config.DisableGpu {
		args = append(args, "--disable-gpu")
	}
	if config.DisableWebSec {
		args = append(args, "--disable-web-security")
	}

	// Headless模式
	if !config.Visible && config.Headless {
		args = append(args, "--headless=new")
	}

	// 设置代理
	if config.Proxy != "" {
		args = append(args, "--proxy-server="+config.Proxy)
	}

	// 设置用户代理
	if config.UserAgent != "" {
		args = append(args, "--user-agent="+config.UserAgent)
	}

	cmd := exec.Command(chromePath, args...)

	// 在新进程中启动Chrome
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动Chrome失败: %w", err)
	}

	// 等待调试端口可用
	maxWait := 20 * time.Second
	startTime := time.Now()

	for time.Since(startTime) < maxWait {
		if r.checkConnection(context.Background(), debugURL) {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("等待Chrome调试端口超时")
}

// buildExecAllocatorOptions 构建执行器选项
func (r *ChromeDPRenderer) buildExecAllocatorOptions(config *models.ChromeDPConfig) []chromedp.ExecAllocatorOption {
	options := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
	}

	// 设置headless/visible
	if !config.Visible && config.Headless {
		options = append(options, chromedp.Flag("headless", "new"))
	}

	// 设置用户数据目录
	if config.UserDataDir != "" {
		if err := os.MkdirAll(config.UserDataDir, 0755); err == nil {
			options = append(options, chromedp.Flag("user-data-dir", config.UserDataDir))
		}
	}

	// 设置配置文件
	if config.Profile != "" {
		options = append(options, chromedp.Flag("profile-directory", config.Profile))
	}

	// 设置代理
	if config.Proxy != "" {
		options = append(options, chromedp.Flag("proxy-server", config.Proxy))
	}

	// 设置用户代理
	if config.UserAgent != "" {
		options = append(options, chromedp.Flag("user-agent", config.UserAgent))
	}

	// 禁用功能
	if config.DisableGpu {
		options = append(options, chromedp.Flag("disable-gpu", true))
	}
	if config.NoSandbox {
		options = append(options, chromedp.Flag("no-sandbox", true))
	}
	if config.DisableDevShm {
		options = append(options, chromedp.Flag("disable-dev-shm-usage", true))
	}
	if config.DisableWebSec {
		options = append(options, chromedp.Flag("disable-web-security", true))
	}
	if config.DisableFeatures != "" {
		options = append(options, chromedp.Flag("disable-features", config.DisableFeatures))
	}

	// 窗口大小
	if config.WindowWidth > 0 && config.WindowHeight > 0 {
		options = append(options, chromedp.Flag("window-size", fmt.Sprintf("%d,%d", config.WindowWidth, config.WindowHeight)))
	}

	return options
}

// buildRenderingActions 构建渲染动作
func (r *ChromeDPRenderer) buildRenderingActions(url string, width, height int, config *models.ChromeDPConfig, htmlContent *string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		// 导航到目标URL
		if err := chromedp.Navigate(url).Do(ctx); err != nil {
			return err
		}

		// 设置视口大小
		if err := chromedp.EmulateViewport(int64(width), int64(height)).Do(ctx); err != nil {
			return err
		}

		// 等待页面加载完成
		if err := chromedp.WaitReady("body", chromedp.ByQuery).Do(ctx); err != nil {
			return err
		}

		// 额外等待动态内容加载
		waitDelay := config.WaitDelay
		if waitDelay <= 0 {
			waitDelay = 2
		}
		if err := chromedp.Sleep(time.Duration(waitDelay) * time.Second).Do(ctx); err != nil {
			return err
		}

		// 获取页面HTML
		if err := chromedp.OuterHTML("html", htmlContent).Do(ctx); err != nil {
			return err
		}

		return nil
	})
}

// ValidateConfig 验证配置
func (r *ChromeDPRenderer) ValidateConfig(config *models.ChromeDPConfig) error {
	// 检查ChromeDP配置
	if !config.Enabled {
		return fmt.Errorf("ChromeDP渲染器未启用")
	}

	// 验证URL格式（如果配置了RemoteDebugURL）
	if config.RemoteDebugURL != "" {
		if !strings.HasPrefix(config.RemoteDebugURL, "http://") &&
			!strings.HasPrefix(config.RemoteDebugURL, "https://") {
			return fmt.Errorf("ChromeDP RemoteDebugURL格式无效，应以http://或https://开头")
		}
	}

	// 如果配置了Chrome路径，检查文件是否存在
	if config.ChromePath != "" {
		if _, err := os.Stat(config.ChromePath); err != nil {
			return fmt.Errorf("Chrome路径不存在: %s", config.ChromePath)
		}
	}

	// 验证窗口尺寸
	if config.WindowWidth <= 0 {
		return fmt.Errorf("ChromeDP窗口宽度配置无效")
	}

	if config.WindowHeight <= 0 {
		return fmt.Errorf("ChromeDP窗口高度配置无效")
	}

	return nil
}

// GetDefaultOptions 获取默认选项
func (r *ChromeDPRenderer) GetDefaultOptions() map[string]interface{} {
	return map[string]interface{}{
		"window_width":  1920,
		"window_height": 1080,
		"headless":      true,
		"visible":       false,
		"reuse_browser": false,
		"disable_gpu":   true,
		"no_sandbox":    true,
		"wait_timeout":  30,
		"wait_delay":    2,
	}
}

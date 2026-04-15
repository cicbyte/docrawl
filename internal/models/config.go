package models

// AppConfig 应用配置结构
type AppConfig struct {
	Version string `yaml:"version"` // 版本号，用于升级时判断
	AI      AIConfig `yaml:"ai"`
	Log     LogConfig `yaml:"log"`
	Crawler CrawlerConfig `yaml:"crawler"`
}

// AIConfig AI配置
type AIConfig struct {
	Provider    string  `yaml:"provider"`    // openai/ollama/zhipu
	BaseURL     string  `yaml:"base_url"`    // API基础URL
	ApiKey      string  `yaml:"api_key"`     // API密钥
	Model       string  `yaml:"model"`       // 模型名称
	MaxTokens   int     `yaml:"max_tokens"`  // 最大token数
	Temperature float64 `yaml:"temperature"` // 温度参数
	Timeout     int     `yaml:"timeout"`     // 请求超时（秒）
}

// LogConfig 日志配置
type LogConfig struct {
	Level      string `yaml:"level"`       // 日志级别
	MaxSize    int    `yaml:"max_size"`    // 单个日志文件最大大小(MB)
	MaxBackups int    `yaml:"max_backups"` // 保留旧日志文件最大数量
	MaxAge     int    `yaml:"max_age"`     // 保留旧日志文件最大天数
	Compress   bool   `yaml:"compress"`    // 是否压缩旧日志文件
}

// CrawlerConfig 爬虫配置
type CrawlerConfig struct {
	// 并发设置
	Concurrency int `yaml:"concurrency"` // 并发数（默认3）

	// 超时设置
	RequestTimeout int `yaml:"request_timeout"` // 请求超时（秒，默认30）
	PageTimeout    int `yaml:"page_timeout"`    // 页面加载超时（秒，默认60）

	// 重试设置
	MaxRetries int `yaml:"max_retries"` // 最大重试次数（默认3）
	RetryDelay int `yaml:"retry_delay"` // 重试延迟（秒，默认1）

	// 渲染设置
	ChromeDP ChromeDPConfig `yaml:"chromedp"`

	// 输出设置
	SaveRawHTML  bool `yaml:"save_raw_html"`  // 是否保存原始HTML
	IncludeMeta  bool `yaml:"include_meta"`   // 是否包含元数据
}

// ChromeDPConfig ChromeDP配置
type ChromeDPConfig struct {
	// 基本设置
	Enabled    bool   `yaml:"enabled"`     // 是否启用ChromeDP
	Headless   bool   `yaml:"headless"`    // 是否无头模式
	Visible    bool   `yaml:"visible"`     // 是否可视化模式
	ChromePath string `yaml:"chrome_path"` // Chrome可执行文件路径

	// 调试设置
	DebugPort    int    `yaml:"debug_port"`     // 调试端口
	RemoteDebugURL string `yaml:"remote_debug_url"` // 远程调试URL
	ReuseBrowser bool   `yaml:"reuse_browser"` // 是否复用浏览器
	KeepBrowser  bool   `yaml:"keep_browser"`  // 是否保持浏览器

	// 用户数据
	UserDataDir string `yaml:"user_data_dir"` // 用户数据目录
	Profile     string `yaml:"profile"`       // 配置文件名

	// 网络设置
	Proxy    string `yaml:"proxy"`     // 代理服务器
	UserAgent string `yaml:"user_agent"` // 用户代理

	// 窗口设置
	WindowWidth  int `yaml:"window_width"`  // 窗口宽度
	WindowHeight int `yaml:"window_height"` // 窗口高度

	// 渲染设置
	WaitTimeout int `yaml:"wait_timeout"` // 页面等待超时（秒）
	WaitDelay   int `yaml:"wait_delay"`   // 额外等待时间（秒）

	// 安全设置
	NoSandbox      bool `yaml:"no_sandbox"`       // 禁用沙箱
	DisableGpu     bool `yaml:"disable_gpu"`      // 禁用GPU
	DisableDevShm  bool `yaml:"disable_dev_shm"`  // 禁用/dev/shm
	DisableWebSec  bool `yaml:"disable_web_sec"`  // 禁用Web安全
	DisableFeatures string `yaml:"disable_features"` // 禁用的特性
}

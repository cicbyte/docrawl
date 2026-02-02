package utils

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/cicbyte/docrawl/internal/models"
	"go.yaml.in/yaml/v3"
)

var ConfigInstance = Config{}

type Config struct {
	HomeDir      string
	AppSeriesDir string
	AppDir       string
	ConfigDir    string
	ConfigPath   string
	DbDir        string
	DbPath       string
	LogDir       string
	LogPath      string
}

func (c *Config) GetHomeDir() string {
	if c.HomeDir != "" {
		return c.HomeDir
	}
	usr, err := user.Current()
	if err != nil {
		panic(fmt.Sprintf("Failed to get current user: %v", err))
	}
	c.HomeDir = usr.HomeDir
	return c.HomeDir
}

func (c *Config) GetAppSeriesDir() string {
	if c.AppSeriesDir != "" {
		return c.AppSeriesDir
	}
	c.AppSeriesDir = c.GetHomeDir() + "/.cicbyte"
	return c.AppSeriesDir
}

func (c *Config) GetAppDir() string {
	if c.AppDir != "" {
		return c.AppDir
	}
	c.AppDir = c.GetAppSeriesDir() + "/docrawl"
	return c.AppDir
}

func (c *Config) GetConfigDir() string {
	if c.ConfigDir != "" {
		return c.ConfigDir
	}
	c.ConfigDir = c.GetAppDir() + "/config"
	return c.ConfigDir
}
func (c *Config) GetConfigPath() string {
	if c.ConfigPath != "" {
		return c.ConfigPath
	}
	c.ConfigPath = c.GetConfigDir() + "/config.yaml"
	return c.ConfigPath
}

func (c *Config) GetDbDir() string {
	if c.DbDir != "" {
		return c.DbDir
	}
	dbDir := filepath.Join(c.GetAppDir(), "db")
	c.DbDir = dbDir
	return c.DbDir
}

func (c *Config) GetDbPath() string {
	if c.DbPath != "" {
		return c.DbPath
	}
	c.DbPath = filepath.Join(c.GetDbDir(), "app.db")
	return c.DbPath
}

func (c *Config) GetLogDir() string {
	if c.LogDir == "" {
		c.LogDir = filepath.Join(c.GetAppDir(), "logs")
	}
	return c.LogDir
}

func (c *Config) GetLogPath() string {
	if c.LogPath == "" {
		now := time.Now().Format("20060102")
		c.LogPath = filepath.Join(c.GetLogDir(), fmt.Sprintf("docrawl_log_%s.log", now))
	}
	return c.LogPath
}

func (c *Config) LoadConfig() *models.AppConfig {
	config_path := c.GetConfigPath()

	// 检查配置文件是否存在
	if _, err := os.Stat(config_path); os.IsNotExist(err) {
		defaultConfig := GetDefaultConfig()
		// 将默认配置写入文件
		data, err := yaml.Marshal(defaultConfig)
		if err == nil {
			_ = os.WriteFile(config_path, data, 0644)
		}
		return defaultConfig
	}

	// 读取配置文件内容
	data, err := os.ReadFile(config_path)
	if err != nil {
		return GetDefaultConfig()
	}

	// 解析YAML配置
	var config models.AppConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return GetDefaultConfig()
	}

	return &config
}

func (c *Config) SaveConfig(config *models.AppConfig) {
	config_path := c.GetConfigPath()
	data, err := yaml.Marshal(config)
	if err != nil {
		return
	}
	os.WriteFile(config_path, data, 0644)
}

// 默认配置
func GetDefaultConfig() *models.AppConfig {
	config := &models.AppConfig{}

	// AI配置默认值
	config.AI.Provider = "openai"
	// 默认使用智谱AI的GLM-4-Flash-250414模型
	config.AI.BaseURL = "https://open.bigmodel.cn/api/paas/v4/"
	config.AI.Model = "GLM-4-Flash-250414"
	config.AI.MaxTokens = 2048
	config.AI.Temperature = 0.8
	config.AI.Timeout = 30

	// 日志默认配置
	config.Log.Level = "info"
	config.Log.MaxSize = 10
	config.Log.MaxBackups = 30
	config.Log.MaxAge = 30
	config.Log.Compress = true

	// 爬虫默认配置
	config.Crawler.Concurrency = 3
	config.Crawler.RequestTimeout = 30
	config.Crawler.PageTimeout = 60
	config.Crawler.MaxRetries = 3
	config.Crawler.RetryDelay = 1
	config.Crawler.SaveRawHTML = false
	config.Crawler.IncludeMeta = true

	// ChromeDP默认配置
	config.Crawler.ChromeDP.Enabled = true
	config.Crawler.ChromeDP.Headless = true
	config.Crawler.ChromeDP.Visible = false
	config.Crawler.ChromeDP.DebugPort = 9222
	config.Crawler.ChromeDP.WindowWidth = 1920
	config.Crawler.ChromeDP.WindowHeight = 1080
	config.Crawler.ChromeDP.WaitTimeout = 30
	config.Crawler.ChromeDP.WaitDelay = 2
	config.Crawler.ChromeDP.NoSandbox = true
	config.Crawler.ChromeDP.DisableGpu = true
	config.Crawler.ChromeDP.DisableDevShm = true
	config.Crawler.ChromeDP.DisableWebSec = true

	return config
}

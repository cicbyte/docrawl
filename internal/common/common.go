package common

import (
	"embed"
	"io/fs"

	"github.com/cicbyte/docrawl/internal/models"
)

var (
	AppConfigModel *models.AppConfig
	AssetsFS       embed.FS // 嵌入的资源文件系统
)

// GetAssetFile 获取嵌入的资源文件内容
func GetAssetFile(path string) ([]byte, error) {
	return AssetsFS.ReadFile(path)
}

// AssetExists 检查嵌入的资源文件是否存在
func AssetExists(path string) bool {
	_, err := fs.Stat(AssetsFS, path)
	return err == nil
}

package utils

import (
	"fmt"
	"sync"

	"github.com/cicbyte/docrawl/internal/log"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var (
	gormDB *gorm.DB
	dbOnce sync.Once
)

type DBConfig struct {
	Type     string
	Host     string
	Port     int
	User     string
	Password string
	DbName   string
}

func GetGormDB() (*gorm.DB, error) {
	var err error
	dbOnce.Do(func() {
		cInstance := ConfigInstance
		config := cInstance.LoadConfig()

		dbConfig := DBConfig{
			Type:     config.Database.Type,
			Host:     config.Database.Host,
			Port:     config.Database.Port,
			User:     config.Database.User,
			Password: config.Database.Password,
			DbName:   config.Database.DbName,
		}

		gormDB, err = initGormDB(dbConfig)
	})
	//err = gormDB.AutoMigrate()
	if err != nil {
		return nil, err
	}
	return gormDB, err
}

func initGormDB(config DBConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch config.Type {
	case "mysql":
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			config.User, config.Password, config.Host, config.Port, config.DbName)
		dialector = mysql.Open(dsn)
	case "sqlite":
		dbPath := ConfigInstance.GetDbPath()
		dialector = sqlite.Open(dbPath)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", config.Type)
	}

	return gorm.Open(dialector, &gorm.Config{
		Logger: log.GetGormLogger(),
	})
}

func CloseGormDB() error {
	if gormDB != nil {
		sqlDB, err := gormDB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}
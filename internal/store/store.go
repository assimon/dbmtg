// Package store 封装数据库访问。
package store

import (
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/assimon/dbmtg/internal/model"
)

type Store struct {
	db *gorm.DB
}

// Open 打开（必要时创建）SQLite 数据库并自动迁移表结构
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(model.All()...); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

// first 查询第一条记录，不存在时返回 nil 而不是错误
func first[T any](q *gorm.DB) (*T, error) {
	var rows []T
	if err := q.Limit(1).Find(&rows).Error; err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

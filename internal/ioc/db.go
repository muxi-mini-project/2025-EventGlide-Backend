package ioc

import (
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func InitDB(cfg *config.Conf) *gorm.DB {
	model.SetDecryptErrorLogf(func(format string, args ...interface{}) {
		logger.GetLogger("bff").Warn(fmt.Sprintf(format, args...))
	})

	gormLogger := gormlogger.New(
		log.New(log.Writer(), "\r\n", log.LstdFlags),
		gormlogger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  gormlogger.Warn,
			IgnoreRecordNotFoundError: true,
		},
	)

	db, err := gorm.Open(mysql.Open(cfg.Mysql.DSN), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   gormLogger,
	})
	if err != nil {
		log.Fatalln(err)
	}
	sqldb, err := db.DB()
	if err != nil {
		log.Fatalln(err)
	}
	sqldb.SetMaxIdleConns(cfg.Mysql.MaxIdleConns)
	sqldb.SetMaxOpenConns(cfg.Mysql.MaxOpenConns)

	err = migrate(db)
	if err != nil {
		log.Fatalln(err)
	}

	return db
}

func migrate(db *gorm.DB) error {
	// 三张互动表将建 (user_id, xxx_id, type) 唯一索引，存量重复行会导致建索引失败、服务无法启动。
	// 建索引前先程序化去重（保留最小 id，幂等），杜绝启动崩溃。
	if err := dedupInteractions(db); err != nil {
		return err
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.Activity{},
		&model.ActivityDraft{},
		&model.ActivitySigner{},
		&model.Comment{},
		&model.Post{},
		&model.PostDraft{},
		&model.Approvement{},
		&model.AuditorForm{},
		&model.Image{},
		&model.UserActivityInteraction{},
		&model.UserPostInteraction{},
		&model.UserCommentInteraction{},
	); err != nil {
		return err
	}

	return db.AutoMigrate(&model.Feed{})
}

// dedupInteractions 清理互动表重复行（保留每组重复中 id 最小的一条），保证唯一索引可建。
// 全新库表尚未由 AutoMigrate 创建，跳过不存在的表。
func dedupInteractions(db *gorm.DB) error {
	type dedup struct {
		Table  string
		Target string
	}
	tables := []dedup{
		{Table: "user_activity_interaction", Target: "activity_id"},
		{Table: "user_post_interaction", Target: "post_id"},
		{Table: "user_comment_interaction", Target: "comment_id"},
	}
	for _, t := range tables {
		var exists bool
		if err := db.Raw(
			"SELECT COUNT(*) > 0 FROM information_schema.tables "+
				"WHERE table_schema = DATABASE() AND table_name = ?", t.Table,
		).Scan(&exists).Error; err != nil {
			return err
		}
		if !exists {
			continue
		}
		res := db.Exec(fmt.Sprintf(
			"DELETE t1 FROM %s t1 JOIN %s t2 "+
				"ON t1.user_id=t2.user_id AND t1.%s=t2.%s AND t1.type=t2.type AND t1.id>t2.id",
			t.Table, t.Table, t.Target, t.Target,
		))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected > 0 {
			log.Printf("dedupInteractions: removed %d duplicate rows from %s\n", res.RowsAffected, t.Table)
		}
	}
	return nil
}

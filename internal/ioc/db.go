package ioc

import (
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
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
		zapWriter{logger.GetLogger("bff")},
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
// 去重会少计互动数，随后按去重后的表修正 like_num/collect_num 计数（与目标表已存在时）。
func dedupInteractions(db *gorm.DB) error {
	tables := []dedupTarget{
		{Table: "user_activity_interaction", Target: "activity_id", TargetTbl: "activity", LikeCol: "like_num", CollectCol: "collect_num"},
		{Table: "user_post_interaction", Target: "post_id", TargetTbl: "post", LikeCol: "like_num", CollectCol: "collect_num"},
		{Table: "user_comment_interaction", Target: "comment_id", TargetTbl: "comment", LikeCol: "like_num"},
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
			if err := syncInteractionCounts(db, t); err != nil {
				return err
			}
		}
	}
	return nil
}

// zapWriter 将 GORM 日志转发给 zap 结构化日志，替代标准库 stdout 输出
type zapWriter struct {
	l *zap.Logger
}

func (w zapWriter) Printf(format string, args ...interface{}) {
	w.l.Warn(fmt.Sprintf(format, args...))
}

// dedupTarget 描述一张互动表及其计数同步目标
type dedupTarget struct {
	Table      string
	Target     string
	TargetTbl  string
	LikeCol    string
	CollectCol string
}

// syncInteractionCounts 按互动表重算目标表的 like_num/collect_num，修正去重导致的计数虚高。
// 目标表不存在（旧库尚未建）时跳过。

func syncInteractionCounts(db *gorm.DB, t dedupTarget) error {
	var targetExists bool
	if err := db.Raw(
		"SELECT COUNT(*) > 0 FROM information_schema.tables "+
			"WHERE table_schema = DATABASE() AND table_name = ?", t.TargetTbl,
	).Scan(&targetExists).Error; err != nil {
		return err
	}
	if !targetExists {
		return nil
	}

	// 修正点赞计数
	if err := syncCountColumn(db, t.Table, t.Target, t.TargetTbl, t.LikeCol, "like"); err != nil {
		return err
	}
	// 修正收藏计数（comment 无收藏）
	if t.CollectCol != "" {
		if err := syncCountColumn(db, t.Table, t.Target, t.TargetTbl, t.CollectCol, "collect"); err != nil {
			return err
		}
	}
	return nil
}

// syncCountColumn 用互动表实际行数覆盖目标表的计数字段
func syncCountColumn(db *gorm.DB, table, target, targetTbl, col, typ string) error {
	return db.Exec(fmt.Sprintf(
		"UPDATE %s tgt SET tgt.%s = ("+
			"SELECT COUNT(*) FROM %s i WHERE i.%s = tgt.id AND i.type = '%s'"+
			")",
		targetTbl, col, table, target, typ,
	)).Error
}

package ioc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
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

const migrateLockName = "eg:migrate"

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
		TranslateError:                           true,
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
	// 单实例迁移：用 MySQL advisory lock 串行化，避免多副本同时启动时互相干扰
	// （共享去重临时表冲突、并发建唯一索引）。
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	unlock, err := acquireMigrateLock(ctx, sqlDB)
	if err != nil {
		return err
	}
	defer unlock()

	// 三张互动表将建 (user_id, xxx_id, type) 唯一索引，存量重复行会导致建索引失败、服务无法启动。
	// 建索引前先程序化去重（保留最小 id，幂等），杜绝启动崩溃。
	if err := dedupInteractions(db); err != nil {
		return err
	}

	// auditor_form 将建 (activity_id, subject) 唯一索引，需先清掉存量重复行
	// （历史 worker 每 5s 重复送审累积的），否则 AutoMigrate 建索引失败。
	// 去重后若仍有并发写入造成重复（滚动发布期间旧实例仍在写），建唯一索引会因 1062
	// （TranslateError 后为 gorm.ErrDuplicatedKey）失败；此时重新去重并重试，有界循环，
	// 避免迁移整体失败导致进程退出。已有索引时去重会被短路为廉价空操作。
	const maxMigrateAttempts = 5
	var migrateErr error
	for attempt := 1; attempt <= maxMigrateAttempts; attempt++ {
		if err := dedupAuditorForms(db); err != nil {
			return err
		}
		migrateErr = autoMigrateAll(db)
		if migrateErr == nil {
			break
		}
		if !errors.Is(migrateErr, gorm.ErrDuplicatedKey) {
			return migrateErr
		}
		log.Printf("migrate: duplicate rows during index creation (attempt %d/%d), retrying after dedup\n", attempt, maxMigrateAttempts)
		time.Sleep(500 * time.Millisecond)
	}
	if migrateErr != nil {
		return migrateErr
	}

	return db.AutoMigrate(&model.Feed{})
}

func autoMigrateAll(db *gorm.DB) error {
	return db.AutoMigrate(
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
	)
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

// acquireMigrateLock 用 MySQL advisory lock 串行化迁移，返回释放函数。
// 多副本同时启动时，先拿到锁的实例执行去重/建索引，其余实例等待后再进入（此时已是幂等空操作）。
// GET_LOCK 作用于单条连接，而 sql.DB 是连接池，RELEASE_LOCK 必须回到同一条连接，
// 因此整段占用一条专用连接（conn），释放后再归还。
func acquireMigrateLock(ctx context.Context, sqlDB *sql.DB) (func(), error) {
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, err
	}

	var ok sql.NullBool
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", migrateLockName, 600).Scan(&ok); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if !ok.Valid || !ok.Bool {
		_ = conn.Close()
		return nil, fmt.Errorf("acquire migrate lock %q timed out", migrateLockName)
	}

	return func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", migrateLockName)
		_ = conn.Close()
	}, nil
}

// dedupAuditorForms 清理 auditor_form 中 (activity_id, subject) 的重复行，保证唯一索引可建。
// 每个分组保留一行：优先保留已有审核结论（pass/reject）的行，其次保留 id 最小（最早创建）的行。
// 表不存在（全新库）或唯一索引已存在时跳过，避免每次启动做全表扫描。
// 先一次性算出保留集与快照上界（避免每批重算窗口函数、且不误删快照后新增的行），
// 再按上界分批删除，兼顾 179 万级数据下的启动耗时与锁表风险。
func dedupAuditorForms(db *gorm.DB) error {
	var exists bool
	if err := db.Raw(
		"SELECT COUNT(*) > 0 FROM information_schema.tables " +
			"WHERE table_schema = DATABASE() AND table_name = 'auditor_form'",
	).Scan(&exists).Error; err != nil {
		return err
	}
	if !exists {
		return nil
	}

	// 已有唯一索引即认为是干净状态，直接跳过，避免每次启动全表扫描。
	var hasIndex bool
	if err := db.Raw(
		"SELECT COUNT(*) > 0 FROM information_schema.statistics " +
			"WHERE table_schema = DATABASE() AND table_name = 'auditor_form' " +
			"AND index_name = 'idx_auditor_form_act_subject'",
	).Scan(&hasIndex).Error; err != nil {
		return err
	}
	if hasIndex {
		return nil
	}

	// 快照上界：只处理此刻已存在的行，避免删除并发新增的合法行。
	var maxID int64
	if err := db.Raw("SELECT COALESCE(MAX(id), 0) FROM auditor_form").Scan(&maxID).Error; err != nil {
		return err
	}

	const keepTable = "auditor_form_dedup_keep"
	if err := db.Exec("DROP TABLE IF EXISTS " + keepTable).Error; err != nil {
		return err
	}
	defer db.Exec("DROP TABLE IF EXISTS " + keepTable)

	if err := db.Exec(fmt.Sprintf(
		`CREATE TABLE %s (id BIGINT NOT NULL PRIMARY KEY) AS
		 SELECT id FROM (
		   SELECT id,
		          ROW_NUMBER() OVER (
		            PARTITION BY activity_id, subject
		            ORDER BY (status IN ('pass','reject')) DESC, id ASC
		          ) rn
		   FROM auditor_form
		   WHERE id <= %d
		 ) x WHERE rn = 1`, keepTable, maxID,
	)).Error; err != nil {
		return err
	}

	const batchSize = 10000
	total := int64(0)
	for {
		res := db.Exec(fmt.Sprintf(
			"DELETE FROM auditor_form WHERE id <= %d AND id NOT IN (SELECT id FROM %s) LIMIT %d",
			maxID, keepTable, batchSize,
		))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			break
		}
		total += res.RowsAffected
		time.Sleep(100 * time.Millisecond)
	}
	if total > 0 {
		log.Printf("dedupAuditorForms: removed %d duplicate rows from auditor_form\n", total)
	}
	return nil
}

// zapWriter 将 GORM 日志转发给 zap 结构化日志，替代标准库 stdout 输出。
// 分流规则依赖 LogLevel=Warn 的配置（InitDB 中 gormlogger.Config）：
// 在该级别下 GORM 只会输出四类消息——
//   - Error()/Warn() 方法：带 "[error] " / "[warn] " 前缀
//   - Trace 慢 SQL：带 "SLOW SQL >=" 字样
//   - Trace 错误 SQL：无级别前缀
//
// 故带 "[warn] " 或 "SLOW SQL" 判为 Warn，其余（含 "[error] " 与无前缀的错误 trace）判为 Error。
// 若将来调整 LogLevel 为 Info，需同步修正此处（普通 trace 无前缀会被误判 Error）。
type zapWriter struct {
	l *zap.Logger
}

func (w zapWriter) Printf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if strings.Contains(msg, "[warn] ") || strings.Contains(msg, "SLOW SQL") {
		w.l.Warn(msg)
		return
	}
	w.l.Error(msg)
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

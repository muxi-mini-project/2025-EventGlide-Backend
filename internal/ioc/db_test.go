package ioc

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestConnLifetimeOrDefaults(t *testing.T) {
	tests := []struct {
		name         string
		lifetime     time.Duration
		idle         time.Duration
		wantLifetime time.Duration
		wantIdle     time.Duration
	}{
		{"zero falls back to defaults", 0, 0, defaultConnMaxLifetime, defaultConnMaxIdleTime},
		{"negative falls back to defaults", -time.Second, -time.Minute, defaultConnMaxLifetime, defaultConnMaxIdleTime},
		{"sub-second misparse falls back to defaults", 3600 * time.Nanosecond, 500 * time.Millisecond, defaultConnMaxLifetime, defaultConnMaxIdleTime},
		{"only lifetime set defaults idle", 30 * time.Minute, 0, 30 * time.Minute, defaultConnMaxIdleTime},
		{"only idle set defaults lifetime", 0, 5 * time.Minute, defaultConnMaxLifetime, 5 * time.Minute},
		{"explicit values pass through", 30 * time.Minute, 5 * time.Minute, 30 * time.Minute, 5 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLifetime, gotIdle := connLifetimeOrDefaults(tt.lifetime, tt.idle)
			if gotLifetime != tt.wantLifetime || gotIdle != tt.wantIdle {
				t.Fatalf("connLifetimeOrDefaults(%v, %v) = (%v, %v), want (%v, %v)",
					tt.lifetime, tt.idle, gotLifetime, gotIdle, tt.wantLifetime, tt.wantIdle)
			}
		})
	}
}

func newDBForMigrationTest(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}

// TestRenameLegacyActivityUniqueIndex 锁住迁移的四条分支：全新库/无索引不动手，
// 旧索引（不含 revision）被改名让位，已迁移索引保持不动（重复启动幂等），
// 改名后中断（idx_activity_unique 不存在）时留给 AutoMigrate 建新索引。
func TestRenameLegacyActivityUniqueIndex(t *testing.T) {
	const (
		tableQuery = "SELECT COUNT\\(\\*\\) > 0 FROM information_schema.tables WHERE table_schema = DATABASE\\(\\) AND table_name = \\?"
		indexQuery = "SELECT COUNT\\(\\*\\) > 0 FROM information_schema.statistics WHERE table_schema = DATABASE\\(\\) AND table_name = \\? AND index_name = \\?"
		colQuery   = "SELECT COUNT\\(\\*\\) > 0 FROM information_schema.statistics WHERE table_schema = DATABASE\\(\\) AND table_name = \\? AND index_name = \\? AND column_name = 'revision'"
		renameStmt = "ALTER TABLE activity RENAME INDEX idx_activity_unique TO idx_activity_unique_legacy"
	)

	t.Run("全新库表不存在时跳过", func(t *testing.T) {
		db, mock := newDBForMigrationTest(t)
		mock.ExpectQuery(tableQuery).WithArgs("activity").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		if err := renameLegacyActivityUniqueIndex(db); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("索引不存在时跳过", func(t *testing.T) {
		db, mock := newDBForMigrationTest(t)
		mock.ExpectQuery(tableQuery).WithArgs("activity").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(indexQuery).WithArgs("activity", "idx_activity_unique").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		if err := renameLegacyActivityUniqueIndex(db); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("旧索引不含 revision 时改名", func(t *testing.T) {
		db, mock := newDBForMigrationTest(t)
		mock.ExpectQuery(tableQuery).WithArgs("activity").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(indexQuery).WithArgs("activity", "idx_activity_unique").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(colQuery).WithArgs("activity", "idx_activity_unique").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec(regexp.QuoteMeta(renameStmt)).WillReturnResult(sqlmock.NewResult(0, 0))

		if err := renameLegacyActivityUniqueIndex(db); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("索引已含 revision 时跳过", func(t *testing.T) {
		db, mock := newDBForMigrationTest(t)
		mock.ExpectQuery(tableQuery).WithArgs("activity").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(indexQuery).WithArgs("activity", "idx_activity_unique").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(colQuery).WithArgs("activity", "idx_activity_unique").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		if err := renameLegacyActivityUniqueIndex(db); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

// TestDropLegacyActivityUniqueIndex 新索引建成后清理改名保留的旧索引；不存在时为空操作。
func TestDropLegacyActivityUniqueIndex(t *testing.T) {
	const (
		indexQuery = "SELECT COUNT\\(\\*\\) > 0 FROM information_schema.statistics WHERE table_schema = DATABASE\\(\\) AND table_name = \\? AND index_name = \\?"
		dropStmt   = "DROP INDEX idx_activity_unique_legacy ON activity"
	)

	t.Run("旧索引不存在时跳过", func(t *testing.T) {
		db, mock := newDBForMigrationTest(t)
		mock.ExpectQuery(indexQuery).WithArgs("activity", "idx_activity_unique_legacy").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		if err := dropLegacyActivityUniqueIndex(db); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("旧索引存在时删除", func(t *testing.T) {
		db, mock := newDBForMigrationTest(t)
		mock.ExpectQuery(indexQuery).WithArgs("activity", "idx_activity_unique_legacy").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectExec(regexp.QuoteMeta(dropStmt)).WillReturnResult(sqlmock.NewResult(0, 0))

		if err := dropLegacyActivityUniqueIndex(db); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

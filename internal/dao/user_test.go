package dao

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/encrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var _ driver.Valuer = model.EncryptedString("")

// encryptedArgMatcher 断言 SQL 参数是带 v1: 前缀的密文，且可解密回期望明文。
type encryptedArgMatcher struct {
	plain string
}

func (m encryptedArgMatcher) Match(v driver.Value) bool {
	s, ok := v.(string)
	if !ok || !strings.HasPrefix(s, encrypt.Prefix) {
		return false
	}
	dec, err := encrypt.Decrypt(s)
	return err == nil && dec == m.plain
}

func TestUpdateRealNamePassesEncryptedValue(t *testing.T) {
	t.Setenv("EG_PII_KEY", "0123456789abcdef0123456789abcdef")

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	ud := &UserDao{db: gdb}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `user` SET `real_name`=\\? WHERE student_id = \\?").
		WithArgs(encryptedArgMatcher{plain: "张三"}, "2021000001").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := ud.UpdateRealName(context.Background(), "2021000001", "张三"); err != nil {
		t.Fatal(err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

package service

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// fakeImageNotifier 记录创建路径是否唤醒了宽高回填 worker。
type fakeImageNotifier struct {
	notifies int
}

func (f *fakeImageNotifier) Notify() { f.notifies++ }

func newActivityServiceForTest(t *testing.T) (*ActivityService, sqlmock.Sqlmock, *fakeImageNotifier) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		NamingStrategy:         schema.NamingStrategy{SingularTable: true},
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	actDao := dao.NewActDao(gdb, &config.Conf{}, &logger.LoggerSet{Activity: zap.NewNop()})
	notifier := &fakeImageNotifier{}
	return &ActivityService{
		ad:        repo.NewActivityRepo(actDao, nil),
		imgWorker: notifier,
		l:         zap.NewNop(),
	}, mock, notifier
}

// expectActivityInsert 事务内：清草稿 -> 插活动 -> 插图片。
func expectActivityInsert(mock sqlmock.Sqlmock, withImage bool) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `activity_draft`").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec("DELETE FROM `activity_draft`").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO `activity`").WillReturnResult(sqlmock.NewResult(1, 1))
	if withImage {
		mock.ExpectExec("INSERT INTO `image`").WillReturnResult(sqlmock.NewResult(1, 1))
	}
	mock.ExpectCommit()
}

// TestCreateActivityNotifiesWhenImagesPending 创建路径必须在 DAO 清空 act.Images 之前判定 pending，
// 否则图片宽高永远没人回填（回归点：事务后 hasPendingImage(nil) 恒为 false）。
func TestCreateActivityNotifiesWhenImagesPending(t *testing.T) {
	as, mock, notifier := newActivityServiceForTest(t)
	expectActivityInsert(mock, true)

	act := &model.Activity{
		StudentID: "S1",
		Title:     "t",
		Images:    []model.Image{{Url: "http://img/a.png", SizeStatus: model.ImageSizePending}},
	}
	if err := as.CreateActivity(context.Background(), act, nil, "S1", nil); err != nil {
		t.Fatalf("CreateActivity error: %v", err)
	}

	if notifier.notifies != 1 {
		t.Errorf("pending 图片创建后唤醒次数 = %d, want 1", notifier.notifies)
	}
	if act.Images != nil {
		t.Errorf("DAO 应清空 act.Images，实际 = %v；pending 判定因此必须在事务前", act.Images)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestCreateActivitySkipsNotifyWhenSizesReady 宽高齐全的图片不应触发回填唤醒。
func TestCreateActivitySkipsNotifyWhenSizesReady(t *testing.T) {
	as, mock, notifier := newActivityServiceForTest(t)
	expectActivityInsert(mock, true)

	act := &model.Activity{
		StudentID: "S1",
		Title:     "t",
		Images:    []model.Image{{Url: "http://img/a.png", Width: 8, Height: 4, SizeStatus: model.ImageSizeOK}},
	}
	if err := as.CreateActivity(context.Background(), act, nil, "S1", nil); err != nil {
		t.Fatalf("CreateActivity error: %v", err)
	}

	if notifier.notifies != 0 {
		t.Errorf("宽高齐全时唤醒次数 = %d, want 0", notifier.notifies)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func newInteractionServiceForTest(t *testing.T) (*InteractionService, sqlmock.Sqlmock) {
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

	ls := logger.NewLoggerSet()
	cfg := &config.Conf{}
	ad := dao.NewActDao(gdb, cfg, ls)
	sg := NewSubjectGetter(repo.NewActivityRepo(ad, nil), nil, nil, cfg)
	return &InteractionService{sg: sg, l: zap.NewNop()}, mock
}

// TestEnsureTargetVisibleRejectsTakenDownActivity 已下架（reject）活动对非申办人按不存在拒绝，
// 避免知道 id 的人继续点赞/收藏并给创建者产生无意义通知。
func TestEnsureTargetVisibleRejectsTakenDownActivity(t *testing.T) {
	svc, mock := newInteractionServiceForTest(t)
	const actID = int64(1001)

	mockActivityVisible(mock, actID, userA, model.StanceReject)
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `approvement` WHERE activity_id = \\? AND student_id = \\?").
		WithArgs(actID, userB).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	err := svc.ensureTargetVisible(context.Background(), SubjectActivity, actID, userB)
	if !errors.Is(err, errs.ErrActivityNotFound) {
		t.Fatalf("expected ErrActivityNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestEnsureTargetVisibleAllowsPublishedActivity 未下架活动对所有人放行，不查签署关系。
func TestEnsureTargetVisibleAllowsPublishedActivity(t *testing.T) {
	svc, mock := newInteractionServiceForTest(t)
	const actID = int64(1002)

	mockActivityVisible(mock, actID, userA, "pass")

	if err := svc.ensureTargetVisible(context.Background(), SubjectActivity, actID, userB); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

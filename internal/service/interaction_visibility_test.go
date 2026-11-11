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
	pd := dao.NewPostDao(gdb, cfg, ls)
	ar := repo.NewActivityRepo(ad, nil)
	pr := repo.NewPostRepo(pd, nil, ls)
	cd := dao.NewCommentDao(gdb, ls)
	sg := NewSubjectGetter(ar, pr, cd, cfg)
	ir := repo.NewInteractionRepo(dao.NewInteractionDao(gdb, ls), nil, ar, pr, nil)
	return &InteractionService{sg: sg, id: ir, l: zap.NewNop()}, mock
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

// mockCommentRow 预置 FindCmtByID 的评论行，携带其根对象类型与 id，供后续可见性校验使用。
func mockCommentRow(mock sqlmock.Sqlmock, commentID int64, rootType string, rootID int64) {
	mock.ExpectQuery("SELECT \\* FROM `comment` WHERE id = \\? ORDER BY `comment`\\.`id` LIMIT \\?").
		WithArgs(commentID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "student_id", "parent_id", "root_id", "subject", "root_object_id", "root_object_type",
		}).AddRow(commentID, userA, rootID, rootID, rootType, rootID, rootType))
}

// TestEnsureTargetVisibleRejectsCommentOnPendingActivity slow 下待审核活动对普通访客不可见，
// 其评论的点赞/收藏同样按不存在拒绝，避免评论 id 直连绕过详情口径。
func TestEnsureTargetVisibleRejectsCommentOnPendingActivity(t *testing.T) {
	svc, mock := newInteractionServiceForTest(t)
	const (
		commentID = int64(7001)
		actID     = int64(1001)
	)

	mockCommentRow(mock, commentID, SubjectActivity, actID)
	mockActivityVisible(mock, actID, userA, "pending_auditor")
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `approvement` WHERE activity_id = \\? AND student_id = \\?").
		WithArgs(actID, userB).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	err := svc.ensureTargetVisible(context.Background(), SubjectComment, commentID, userB)
	if !errors.Is(err, errs.ErrActivityNotFound) {
		t.Fatalf("expected ErrActivityNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestEnsureTargetVisibleAllowsCommentOnPendingActivityForOwnerAndSigner 创建者与签署人
// 仍可对本人待审核活动下的评论点赞/收藏。
func TestEnsureTargetVisibleAllowsCommentOnPendingActivityForOwnerAndSigner(t *testing.T) {
	const (
		commentID = int64(7002)
		actID     = int64(1002)
	)

	t.Run("创建者", func(t *testing.T) {
		svc, mock := newInteractionServiceForTest(t)
		mockCommentRow(mock, commentID, SubjectActivity, actID)
		mockActivityVisible(mock, actID, userA, "pending_auditor")

		if err := svc.ensureTargetVisible(context.Background(), SubjectComment, commentID, userA); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})

	t.Run("签署人", func(t *testing.T) {
		svc, mock := newInteractionServiceForTest(t)
		mockCommentRow(mock, commentID, SubjectActivity, actID)
		mockActivityVisible(mock, actID, userA, "pending_auditor")
		mock.ExpectQuery("SELECT count\\(\\*\\) FROM `approvement` WHERE activity_id = \\? AND student_id = \\?").
			WithArgs(actID, userC).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		if err := svc.ensureTargetVisible(context.Background(), SubjectComment, commentID, userC); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
}

// TestEnsureTargetVisibleCommentNotFound 评论不存在时透传 ErrCommentNotFound。
func TestEnsureTargetVisibleCommentNotFound(t *testing.T) {
	svc, mock := newInteractionServiceForTest(t)
	const commentID = int64(7999)

	mock.ExpectQuery("SELECT \\* FROM `comment` WHERE id = \\? ORDER BY `comment`\\.`id` LIMIT \\?").
		WithArgs(commentID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	err := svc.ensureTargetVisible(context.Background(), SubjectComment, commentID, userB)
	if !errors.Is(err, errs.ErrCommentNotFound) {
		t.Fatalf("expected ErrCommentNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestEnsureTargetVisibleRejectsCommentOnPrivatePost 根对象是他人私密帖时，评论点赞/收藏
// 按帖子不存在拒绝，与帖子自身的互动口径一致。
func TestEnsureTargetVisibleRejectsCommentOnPrivatePost(t *testing.T) {
	svc, mock := newInteractionServiceForTest(t)
	const (
		commentID = int64(7004)
		postID    = int64(2001)
	)

	mockCommentRow(mock, commentID, SubjectPost, postID)
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `post` WHERE id = \\? AND is_checking = \\? AND \\(visibility = \\? OR student_id = \\?\\)").
		WithArgs(postID, "pass", model.PostVisibilityPublic, userB).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	err := svc.ensureTargetVisible(context.Background(), SubjectComment, commentID, userB)
	if !errors.Is(err, errs.ErrPostNotFound) {
		t.Fatalf("expected ErrPostNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestEnsureTargetVisibleAllowsCommentWithoutRootObject 历史评论可能没有根对象类型，
// 无法定位根对象时放行，避免误伤老数据。
func TestEnsureTargetVisibleAllowsCommentWithoutRootObject(t *testing.T) {
	svc, mock := newInteractionServiceForTest(t)
	const commentID = int64(7005)

	mockCommentRow(mock, commentID, "", 0)

	if err := svc.ensureTargetVisible(context.Background(), SubjectComment, commentID, userB); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

package dao

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func newPostDaoForTest(t *testing.T) (*PostDao, sqlmock.Sqlmock) {
	return newPostDaoWithEffectForTest(t, "")
}

func newPostDaoWithEffectForTest(t *testing.T, effect string) (*PostDao, sqlmock.Sqlmock) {
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

	return &PostDao{db: gdb, effect: effect, l: logger.NewLoggerSet().Post}, mock
}

// TestFindPendingAuditorPostsSQL 锁定待送审帖子的筛选契约：
//   - 仍为 checking；
//   - 排除已有已推送审核表单的行（帖子回调前一直 checking，否则每 5s 会把已送审项重扫一遍）；
//   - 预加载图片——worker 依赖完整行（含图片）重建送审请求。
func TestFindPendingAuditorPostsSQL(t *testing.T) {
	dao, mock := newPostDaoForTest(t)

	mock.ExpectQuery("SELECT \\* FROM `post` WHERE is_checking = \\? AND \\(NOT EXISTS \\(SELECT 1 FROM auditor_form af WHERE af.activity_id = post.id AND af.subject = \\? AND af.pushed_at IS NOT NULL\\)\\)").
		WithArgs("checking", "post").
		WillReturnRows(sqlmock.NewRows([]string{"id", "student_id", "title", "introduce", "is_checking"}).
			AddRow(1001, "S20250001", "招新", "正文", "checking"))
	mock.ExpectQuery("SELECT \\* FROM `image` WHERE `image`.`owner_id` = \\?").
		WithArgs(1001).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "owner_type", "url"}))

	posts, err := dao.FindPendingAuditorPosts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(posts) != 1 || posts[0].Id != 1001 {
		t.Fatalf("expected the single checking post, got %+v", posts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestDeletePostRemovesImages 删除帖子须在同一事务内一并清理其图片。
func TestDeletePostRemovesImages(t *testing.T) {
	dao, mock := newPostDaoForTest(t)

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `post` WHERE id = \\? and student_id = \\?").
		WithArgs(int64(1001), "S20250001").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT `id` FROM `comment` WHERE root_object_id = \\? AND root_object_type = \\?").
		WithArgs(int64(1001), "post").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec("DELETE FROM `comment` WHERE root_object_id = \\? AND root_object_type = \\?").
		WithArgs(int64(1001), "post").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM `user_post_interaction` WHERE post_id = \\?").
		WithArgs(int64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM `image` WHERE owner_type = \\? AND owner_id IN \\(\\?\\)").
		WithArgs("post", int64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	deleted, commentIDs, err := dao.DeletePost(context.Background(), &model.Post{Id: 1001, StudentID: "S20250001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Fatalf("expected delete to report hit")
	}
	if len(commentIDs) != 0 {
		t.Fatalf("expected no comment ids, got %v", commentIDs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestDeletePostCascadesCommentsAndInteractions 删帖须连同该帖的评论、评论互动、帖子互动一并清理，
// 否则留下孤儿评论与指向死帖的点赞/收藏行。
func TestDeletePostCascadesCommentsAndInteractions(t *testing.T) {
	dao, mock := newPostDaoForTest(t)

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `post` WHERE id = \\? and student_id = \\?").
		WithArgs(int64(1001), "S20250001").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT `id` FROM `comment` WHERE root_object_id = \\? AND root_object_type = \\?").
		WithArgs(int64(1001), "post").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7001).AddRow(7002))
	mock.ExpectExec("DELETE FROM `user_comment_interaction` WHERE comment_id IN \\(\\?,\\?\\)").
		WithArgs(int64(7001), int64(7002)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE FROM `comment` WHERE root_object_id = \\? AND root_object_type = \\?").
		WithArgs(int64(1001), "post").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE FROM `user_post_interaction` WHERE post_id = \\?").
		WithArgs(int64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec("DELETE FROM `image` WHERE owner_type = \\? AND owner_id IN \\(\\?\\)").
		WithArgs("post", int64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	deleted, commentIDs, err := dao.DeletePost(context.Background(), &model.Post{Id: 1001, StudentID: "S20250001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Fatalf("expected delete to report hit")
	}
	if len(commentIDs) != 2 || commentIDs[0] != 7001 || commentIDs[1] != 7002 {
		t.Fatalf("expected deleted comment ids [7001 7002], got %v", commentIDs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestDeletePostNotOwnedKeepsImages 帖子不属该学生（删除未命中）时不应触碰图片，
// 否则会越权删除他人帖子的图片。
func TestDeletePostNotOwnedKeepsImages(t *testing.T) {
	dao, mock := newPostDaoForTest(t)

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `post` WHERE id = \\? and student_id = \\?").
		WithArgs(int64(1001), "S-attacker").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	deleted, commentIDs, err := dao.DeletePost(context.Background(), &model.Post{Id: 1001, StudentID: "S-attacker"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commentIDs) != 0 {
		t.Fatalf("non-owned delete must not report comment ids, got %v", commentIDs)
	}
	if deleted {
		t.Fatalf("delete of a post not owned must not report hit")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestDeleteDraftByStudentRemovesImages 删除草稿须清理其 post_draft 图片（草稿保存先删后建，
// 每次保存都重插 image 行）。
func TestDeleteDraftByStudentRemovesImages(t *testing.T) {
	dao, mock := newPostDaoForTest(t)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT `id` FROM `post_draft` WHERE student_id = \\?").
		WithArgs("S20250001").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2001).AddRow(2002))
	mock.ExpectExec("DELETE FROM `image` WHERE owner_type = \\? AND owner_id IN \\(\\?,\\?\\)").
		WithArgs("post_draft", int64(2001), int64(2002)).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec("DELETE FROM `post_draft` WHERE student_id = \\?").
		WithArgs("S20250001").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	err := dao.DB().Transaction(func(tx *gorm.DB) error {
		return dao.DeleteDraftByStudent(context.Background(), tx, "S20250001")
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestSetVisibilityOnlyOwner 只允许作者改自己的帖子：先校验归属（存在且属于本人）再更新。
// 命中返回 true，未命中（不存在或非本人）返回 false 且不改任何行。
func TestSetVisibilityOnlyOwner(t *testing.T) {
	t.Run("hit", func(t *testing.T) {
		dao, mock := newPostDaoForTest(t)
		mock.ExpectQuery("SELECT count\\(\\*\\) FROM `post` WHERE id = \\? AND student_id = \\?").
			WithArgs(int64(1001), "S20250001").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectExec("UPDATE `post` SET `visibility`=\\? WHERE id = \\? AND student_id = \\?").
			WithArgs("private", int64(1001), "S20250001").
			WillReturnResult(sqlmock.NewResult(0, 1))

		updated, err := dao.SetVisibility(context.Background(), 1001, "S20250001", model.PostVisibilityPrivate)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !updated {
			t.Fatalf("expected update hit to report true")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("miss", func(t *testing.T) {
		dao, mock := newPostDaoForTest(t)
		mock.ExpectQuery("SELECT count\\(\\*\\) FROM `post` WHERE id = \\? AND student_id = \\?").
			WithArgs(int64(1001), "S-attacker").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		updated, err := dao.SetVisibility(context.Background(), 1001, "S-attacker", model.PostVisibilityPrivate)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updated {
			t.Fatalf("non-owner must not report hit")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	// 目标值与原值相同（幂等重试）时 MySQL 影响 0 行，但归属命中即应视为成功。
	t.Run("same-value", func(t *testing.T) {
		dao, mock := newPostDaoForTest(t)
		mock.ExpectQuery("SELECT count\\(\\*\\) FROM `post` WHERE id = \\? AND student_id = \\?").
			WithArgs(int64(1001), "S20250001").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectExec("UPDATE `post` SET `visibility`=\\? WHERE id = \\? AND student_id = \\?").
			WithArgs("public", int64(1001), "S20250001").
			WillReturnResult(sqlmock.NewResult(0, 0))

		updated, err := dao.SetVisibility(context.Background(), 1001, "S20250001", model.PostVisibilityPublic)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !updated {
			t.Fatalf("idempotent re-set by owner must report success")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

// TestIsPostVisibleToSQL 可见性判定条件为"公开 或 本人发布"，供互动写入兜底使用。
func TestIsPostVisibleToSQL(t *testing.T) {
	dao, mock := newPostDaoForTest(t)

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `post` WHERE id = \\? AND \\(visibility = \\? OR student_id = \\?\\)").
		WithArgs(int64(1001), model.PostVisibilityPublic, "S_viewer").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	visible, err := dao.IsPostVisibleTo(context.Background(), 1001, "S_viewer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !visible {
		t.Fatalf("expected visible")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestIsPostVisibleToSlowEffectAddsAuditFilter slow 模式下可见性判定须叠加审核过滤（is_checking='pass'），
// 使历史 reject 帖不可被 id 直达、也不被互动写兜底放行。
func TestIsPostVisibleToSlowEffectAddsAuditFilter(t *testing.T) {
	dao, mock := newPostDaoWithEffectForTest(t, "slow")

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `post` WHERE id = \\? AND is_checking = \\? AND \\(visibility = \\? OR student_id = \\?\\)").
		WithArgs(int64(1001), "pass", model.PostVisibilityPublic, "S_viewer").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	visible, err := dao.IsPostVisibleTo(context.Background(), 1001, "S_viewer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if visible {
		t.Fatalf("rejected post must not be visible under slow effect")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

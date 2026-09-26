package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func newFeedServiceForListTest(t *testing.T) (*FeedService, sqlmock.Sqlmock) {
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
	fd := dao.NewFeedDao(gdb, ls)
	ud := repo.NewUserRepo(dao.NewUserDao(gdb, ls), nil) // GetUserInfo 直接查库，不用缓存
	return &FeedService{fd: fd, ud: ud, l: zap.NewNop()}, mock
}

// TestFeedListTargetDeletedFlag 四类 feed 列表方法：目标帖子已删时，返回详情应同时带
// Message="帖子已不存在" 与 TargetDeleted=true（覆盖目标解析与 targets[i] 索引绑定）。
func TestFeedListTargetDeletedFlag(t *testing.T) {
	const (
		viewer  = "S_viewer"
		actor   = "S_actor"
		postID  = int64(1001)
		cmtID   = int64(9009)
		feedSQL = "SELECT \\* FROM `feed` WHERE receiver = \\? and action = \\? and student_id != \\? ORDER BY created_at ASC, id ASC"
		userSQL = "SELECT \\* FROM `user` WHERE student_id = \\? ORDER BY `user`.`id` LIMIT \\?"
		postSQL = "SELECT `id` FROM `post` WHERE id IN \\(\\?\\)"
		imgSQL  = "SELECT `url` FROM `image` WHERE owner_id = \\? AND owner_type = \\? ORDER BY id ASC LIMIT \\?"
	)

	cases := []struct {
		name     string
		action   string
		object   string
		targetID int64
		rootID   int64
		rootType string
		run      func(*FeedService) (deleted bool, msg string, err error)
	}{
		{"like", "like", SubjectPost, postID, 0, "", func(fs *FeedService) (bool, string, error) {
			d, err := fs.GetLikeFeed(context.Background(), viewer)
			if err != nil || len(d) == 0 {
				return false, "", err
			}
			return d[0].TargetDeleted, d[0].Message, nil
		}},
		{"collect", "collect", SubjectPost, postID, 0, "", func(fs *FeedService) (bool, string, error) {
			d, err := fs.GetCollectFeed(context.Background(), viewer)
			if err != nil || len(d) == 0 {
				return false, "", err
			}
			return d[0].TargetDeleted, d[0].Message, nil
		}},
		{"comment", "comment", SubjectComment, cmtID, postID, SubjectPost, func(fs *FeedService) (bool, string, error) {
			d, err := fs.GetCommentFeed(context.Background(), viewer)
			if err != nil || len(d) == 0 {
				return false, "", err
			}
			return d[0].TargetDeleted, d[0].Message, nil
		}},
		{"at", "at", SubjectComment, cmtID, postID, SubjectPost, func(fs *FeedService) (bool, string, error) {
			d, err := fs.GetAtFeed(context.Background(), viewer)
			if err != nil || len(d) == 0 {
				return false, "", err
			}
			return d[0].TargetDeleted, d[0].Message, nil
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs, mock := newFeedServiceForListTest(t)

			mock.ExpectQuery(feedSQL).WithArgs(viewer, tc.action, viewer).
				WillReturnRows(sqlmock.NewRows([]string{"id", "target_id", "root_id", "root_type", "object", "student_id", "receiver", "created_at", "action", "status"}).
					AddRow(1, tc.targetID, tc.rootID, tc.rootType, tc.object, actor, viewer, time.Now(), tc.action, "未读"))
			// resolveFeedTargets 先批量判断目标帖子是否存在：已删返回空
			mock.ExpectQuery(postSQL).WithArgs(postID).
				WillReturnRows(sqlmock.NewRows([]string{"id"}))
			mock.ExpectQuery(userSQL).WithArgs(actor, 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "college", "student_id", "name", "real_name", "avatar", "school"}).
					AddRow(1, "", actor, "张三", "", "http://avatar", ""))
			// 帖子图片也随级联删除
			mock.ExpectQuery(imgSQL).WithArgs(postID, "post", 1).
				WillReturnRows(sqlmock.NewRows([]string{"url"}))

			deleted, msg, err := tc.run(fs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !deleted {
				t.Fatalf("expected TargetDeleted=true for a deleted target post")
			}
			if msg != feedTargetDeletedMsg {
				t.Fatalf("expected message %q, got %q", feedTargetDeletedMsg, msg)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestResolveFeedTargetsBatchesPostExistence 一次批量查询判断所有 feed 目标帖子是否存在，
// 而不是逐条 COUNT（feed 列表 N+1 回归点）。activity 目标不参与查询。
func TestResolveFeedTargetsBatchesPostExistence(t *testing.T) {
	fs, _, mock := newFeedServiceForTest(t)
	feeds := []*model.Feed{
		{Object: SubjectPost, TargetId: 1001},
		{Object: SubjectComment, TargetId: 9009, RootType: SubjectPost, RootID: 1002},
		{Object: SubjectActivity, TargetId: 3003},
	}

	mock.ExpectQuery("SELECT `id` FROM `post` WHERE id IN \\(\\?,\\?\\)").
		WithArgs(int64(1001), int64(1002)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1001))

	targets, deleted := fs.resolveFeedTargets(context.Background(), feeds)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	if targets[0].postID != 1001 || targets[1].postID != 1002 || targets[2].postID != 0 {
		t.Fatalf("unexpected targets: %+v", targets)
	}
	if deleted[1001] {
		t.Fatalf("1001 exists, must not be marked deleted")
	}
	if !deleted[1002] {
		t.Fatalf("1002 missing, must be marked deleted")
	}
}

// TestResolveFeedTargetsDedupsPostIDs 同一帖子被多条 feed 引用时，存在性查询只带去重后的 id。
func TestResolveFeedTargetsDedupsPostIDs(t *testing.T) {
	fs, _, mock := newFeedServiceForTest(t)
	feeds := []*model.Feed{
		{Object: SubjectPost, TargetId: 1001},
		{Object: SubjectComment, TargetId: 9009, RootType: SubjectPost, RootID: 1001},
	}

	mock.ExpectQuery("SELECT `id` FROM `post` WHERE id IN \\(\\?\\)").
		WithArgs(int64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1001))

	targets, deleted := fs.resolveFeedTargets(context.Background(), feeds)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if targets[0].postID != 1001 || targets[1].postID != 1001 {
		t.Fatalf("unexpected targets: %+v", targets)
	}
	if deleted[1001] {
		t.Fatalf("1001 exists, must not be marked deleted")
	}
}

// TestResolveFeedTargetsNoPostSkipsQuery 没有任何帖子目标时不发起存在性查询。
func TestResolveFeedTargetsNoPostSkipsQuery(t *testing.T) {
	fs, _, mock := newFeedServiceForTest(t)
	feeds := []*model.Feed{{Object: SubjectActivity, TargetId: 3003}}

	// 注册一个匹配任意 SQL 且不应被消费的期望：任何查询（无论形状）都会消费它，ExpectationsWereMet 将返回 nil。
	mock.ExpectQuery(".*").WillReturnError(errors.New("no query expected when there are no post targets"))

	_, deleted := fs.resolveFeedTargets(context.Background(), feeds)
	if len(deleted) != 0 {
		t.Fatalf("expected empty deleted set, got %+v", deleted)
	}
	if mock.ExpectationsWereMet() == nil {
		t.Fatalf("post-existence query must be skipped when there are no post targets")
	}
}

// TestFeedTargetDeleted 只有指向帖子且该帖已删时为 true；非帖子目标（postID=0）恒为 false。
func TestFeedTargetDeleted(t *testing.T) {
	deleted := map[int64]bool{1001: true}

	if !feedTargetDeleted(feedTarget{postID: 1001}, deleted) {
		t.Fatalf("deleted post target should be true")
	}
	if feedTargetDeleted(feedTarget{postID: 1002}, deleted) {
		t.Fatalf("still-existing post target should be false")
	}
	if feedTargetDeleted(feedTarget{postID: 0, rootType: SubjectActivity}, deleted) {
		t.Fatalf("non-post target should be false")
	}
}

// TestFeedMessageMarksDeletedPost 目标帖子已删时，feed 文案改为"帖子已不存在"。
func TestFeedMessageMarksDeletedPost(t *testing.T) {
	fs, _, _ := newFeedServiceForTest(t)
	f := &model.Feed{Action: "like", Object: SubjectPost, TargetId: 1001}

	got := fs.feedMessage(f, feedTarget{postID: 1001}, "张三", map[int64]bool{1001: true})
	if got != feedTargetDeletedMsg {
		t.Fatalf("expected %q, got %q", feedTargetDeletedMsg, got)
	}
}

// TestFeedMessageKeepsMessageWhenPostExists 帖子仍在时保留原互动文案。
func TestFeedMessageKeepsMessageWhenPostExists(t *testing.T) {
	fs, _, _ := newFeedServiceForTest(t)
	f := &model.Feed{Action: "like", Object: SubjectPost, TargetId: 1001}

	got := fs.feedMessage(f, feedTarget{postID: 1001}, "张三", map[int64]bool{1001: false})
	if got != "张三赞了你的帖子" {
		t.Fatalf("existing post must keep the original message, got %q", got)
	}
}

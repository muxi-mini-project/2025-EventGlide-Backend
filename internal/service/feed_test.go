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
			// resolveFeedTargets 批量判断目标帖子对 viewer 是否可访问：已删返回空
			mock.ExpectQuery(postSQL).WithArgs(postID, model.PostVisibilityPublic, viewer, "reject").
				WillReturnRows(sqlmock.NewRows([]string{"id"}))
			mock.ExpectQuery(userSQL).WithArgs(actor, 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "college", "student_id", "name", "real_name", "avatar", "school"}).
					AddRow(1, "", actor, "张三", "", "http://avatar", ""))
			// 目标帖已不可访问：不再取图（避免泄露私密/已删帖缩略图）

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

// TestFeedListRejectedActivity 活动被改判不通过（下架）时，四类 feed 都应按"活动已不存在"展示：
// TargetDeleted=true、文案统一替换、首图置空（与帖子被删时的处理保持一致）。
func TestFeedListRejectedActivity(t *testing.T) {
	const (
		viewer  = "S_viewer"
		actor   = "S_actor"
		actID   = int64(3003)
		cmtID   = int64(9009)
		feedSQL = "SELECT \\* FROM `feed` WHERE receiver = \\? and action = \\? and student_id != \\? ORDER BY created_at ASC, id ASC"
		userSQL = "SELECT \\* FROM `user` WHERE student_id = \\? ORDER BY `user`.`id` LIMIT \\?"
		actSQL  = "SELECT `id` FROM `activity` WHERE id IN \\(\\?\\) AND \\(is_checking IS NULL OR is_checking <> \\?\\)"
	)

	cases := []struct {
		name     string
		action   string
		object   string
		targetID int64
		rootID   int64
		rootType string
		run      func(*FeedService) (deleted bool, msg string, pic string, err error)
	}{
		{"like", "like", SubjectActivity, actID, 0, "", func(fs *FeedService) (bool, string, string, error) {
			d, err := fs.GetLikeFeed(context.Background(), viewer)
			if err != nil || len(d) == 0 {
				return false, "", "", err
			}
			return d[0].TargetDeleted, d[0].Message, d[0].FirstPic, nil
		}},
		{"collect", "collect", SubjectActivity, actID, 0, "", func(fs *FeedService) (bool, string, string, error) {
			d, err := fs.GetCollectFeed(context.Background(), viewer)
			if err != nil || len(d) == 0 {
				return false, "", "", err
			}
			return d[0].TargetDeleted, d[0].Message, d[0].FirstPic, nil
		}},
		{"comment", "comment", SubjectComment, cmtID, actID, SubjectActivity, func(fs *FeedService) (bool, string, string, error) {
			d, err := fs.GetCommentFeed(context.Background(), viewer)
			if err != nil || len(d) == 0 {
				return false, "", "", err
			}
			return d[0].TargetDeleted, d[0].Message, d[0].FirstPic, nil
		}},
		{"at", "at", SubjectComment, cmtID, actID, SubjectActivity, func(fs *FeedService) (bool, string, string, error) {
			d, err := fs.GetAtFeed(context.Background(), viewer)
			if err != nil || len(d) == 0 {
				return false, "", "", err
			}
			return d[0].TargetDeleted, d[0].Message, d[0].FirstPic, nil
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs, mock := newFeedServiceForListTest(t)

			mock.ExpectQuery(feedSQL).WithArgs(viewer, tc.action, viewer).
				WillReturnRows(sqlmock.NewRows([]string{"id", "target_id", "root_id", "root_type", "object", "student_id", "receiver", "created_at", "action", "status"}).
					AddRow(1, tc.targetID, tc.rootID, tc.rootType, tc.object, actor, viewer, time.Now(), tc.action, "未读"))
			// resolveFeedTargets 批量判断活动是否仍可展示：已下架（reject）返回空
			mock.ExpectQuery(actSQL).WithArgs(actID, model.StanceReject).
				WillReturnRows(sqlmock.NewRows([]string{"id"}))
			mock.ExpectQuery(userSQL).WithArgs(actor, 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "college", "student_id", "name", "real_name", "avatar", "school"}).
					AddRow(1, "", actor, "张三", "", "http://avatar", ""))
			// 活动已下架：不再取图

			deleted, msg, pic, err := tc.run(fs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !deleted {
				t.Fatalf("expected TargetDeleted=true for a rejected activity")
			}
			if msg != feedActivityDeletedMsg {
				t.Fatalf("expected message %q, got %q", feedActivityDeletedMsg, msg)
			}
			if pic != "" {
				t.Fatalf("rejected activity must not expose first pic, got %q", pic)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestFeedListNormalActivityKeepsMessage 活动仍正常（非 reject）时，文案与首图保持原样。
func TestFeedListNormalActivityKeepsMessage(t *testing.T) {
	const (
		viewer  = "S_viewer"
		actor   = "S_actor"
		actID   = int64(3003)
		feedSQL = "SELECT \\* FROM `feed` WHERE receiver = \\? and action = \\? and student_id != \\? ORDER BY created_at ASC, id ASC"
		userSQL = "SELECT \\* FROM `user` WHERE student_id = \\? ORDER BY `user`.`id` LIMIT \\?"
		actSQL  = "SELECT `id` FROM `activity` WHERE id IN \\(\\?\\) AND \\(is_checking IS NULL OR is_checking <> \\?\\)"
		imgSQL  = "SELECT `url` FROM `image` WHERE owner_id = \\? AND owner_type = \\? ORDER BY id ASC LIMIT \\?"
	)
	fs, mock := newFeedServiceForListTest(t)

	mock.ExpectQuery(feedSQL).WithArgs(viewer, "like", viewer).
		WillReturnRows(sqlmock.NewRows([]string{"id", "target_id", "root_id", "root_type", "object", "student_id", "receiver", "created_at", "action", "status"}).
			AddRow(1, actID, 0, "", SubjectActivity, actor, viewer, time.Now(), "like", "未读"))
	mock.ExpectQuery(actSQL).WithArgs(actID, model.StanceReject).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(actID))
	mock.ExpectQuery(userSQL).WithArgs(actor, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "college", "student_id", "name", "real_name", "avatar", "school"}).
			AddRow(1, "", actor, "张三", "", "http://avatar", ""))
	mock.ExpectQuery(imgSQL).WithArgs(actID, SubjectActivity, 1).
		WillReturnRows(sqlmock.NewRows([]string{"url"}).AddRow("http://pic"))

	d, err := fs.GetLikeFeed(context.Background(), viewer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(d) != 1 {
		t.Fatalf("expected one feed, got %d", len(d))
	}
	if d[0].TargetDeleted {
		t.Fatalf("normal activity must not be marked deleted")
	}
	if d[0].Message != "张三赞了你的活动" {
		t.Fatalf("expected original message, got %q", d[0].Message)
	}
	if d[0].FirstPic != "http://pic" {
		t.Fatalf("expected first pic, got %q", d[0].FirstPic)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestResolveFeedTargetsBatchesPostExistence 一次批量查询判断所有 feed 目标帖子对 viewer 是否可访问，
// 而不是逐条 COUNT（feed 列表 N+1 回归点）。activity 目标走另一条批量查询。
func TestResolveFeedTargetsBatchesPostExistence(t *testing.T) {
	fs, _, mock := newFeedServiceForTest(t)
	const viewer = "S_viewer"
	feeds := []*model.Feed{
		{Object: SubjectPost, TargetId: 1001},
		{Object: SubjectComment, TargetId: 9009, RootType: SubjectPost, RootID: 1002},
		{Object: SubjectActivity, TargetId: 3003},
	}

	mock.ExpectQuery("SELECT `id` FROM `post` WHERE id IN \\(\\?,\\?\\)").
		WithArgs(int64(1001), int64(1002), model.PostVisibilityPublic, viewer, "reject").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1001))
	mock.ExpectQuery("SELECT `id` FROM `activity` WHERE id IN \\(\\?\\)").
		WithArgs(int64(3003), model.StanceReject).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3003))

	targets, deleted := fs.resolveFeedTargets(context.Background(), viewer, feeds)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	if targets[0].postID != 1001 || targets[1].postID != 1002 || targets[2].actID != 3003 {
		t.Fatalf("unexpected targets: %+v", targets)
	}
	if deleted.posts[1001] {
		t.Fatalf("1001 accessible, must not be marked deleted")
	}
	if !deleted.posts[1002] {
		t.Fatalf("1002 inaccessible, must be marked deleted")
	}
	if deleted.activities[3003] {
		t.Fatalf("3003 not rejected, must not be marked deleted")
	}
}

// TestResolveFeedTargetsDedupsPostIDs 同一帖子被多条 feed 引用时，可访问性查询只带去重后的 id。
func TestResolveFeedTargetsDedupsPostIDs(t *testing.T) {
	fs, _, mock := newFeedServiceForTest(t)
	const viewer = "S_viewer"
	feeds := []*model.Feed{
		{Object: SubjectPost, TargetId: 1001},
		{Object: SubjectComment, TargetId: 9009, RootType: SubjectPost, RootID: 1001},
	}

	mock.ExpectQuery("SELECT `id` FROM `post` WHERE id IN \\(\\?\\)").
		WithArgs(int64(1001), model.PostVisibilityPublic, viewer, "reject").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1001))

	targets, deleted := fs.resolveFeedTargets(context.Background(), viewer, feeds)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if targets[0].postID != 1001 || targets[1].postID != 1001 {
		t.Fatalf("unexpected targets: %+v", targets)
	}
	if deleted.posts[1001] {
		t.Fatalf("1001 accessible, must not be marked deleted")
	}
}

// TestResolveFeedTargetsSkipsQueryWithoutTargets 没有帖子/活动目标时不发起对应的可访问性查询。
func TestResolveFeedTargetsSkipsQueryWithoutTargets(t *testing.T) {
	fs, _, mock := newFeedServiceForTest(t)

	// 活动目标：只应有活动查询
	feeds := []*model.Feed{{Object: SubjectActivity, TargetId: 3003}}
	mock.ExpectQuery("SELECT `id` FROM `activity` WHERE id IN \\(\\?\\)").
		WithArgs(int64(3003), model.StanceReject).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3003))

	_, deleted := fs.resolveFeedTargets(context.Background(), "S_viewer", feeds)
	if len(deleted.posts) != 0 {
		t.Fatalf("post query must be skipped when there are no post targets, got %+v", deleted.posts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestResolveFeedTargetsFailClosedOnQueryError 可访问性批量查询失败时必须 fail-closed：
// 把所有目标标为不可访问，避免 DB 抖动时回吐他人私密帖的文案与缩略图、
// 或把状态未知的活动当作正常展示。
func TestResolveFeedTargetsFailClosedOnQueryError(t *testing.T) {
	fs, _, mock := newFeedServiceForTest(t)
	const viewer = "S_viewer"
	feeds := []*model.Feed{
		{Object: SubjectPost, TargetId: 1001},
		{Object: SubjectComment, TargetId: 9009, RootType: SubjectPost, RootID: 1002},
		{Object: SubjectActivity, TargetId: 3003},
	}

	mock.ExpectQuery("SELECT `id` FROM `post` WHERE id IN \\(\\?,\\?\\)").
		WillReturnError(errors.New("db down"))
	mock.ExpectQuery("SELECT `id` FROM `activity` WHERE id IN \\(\\?\\)").
		WillReturnError(errors.New("db down"))

	_, deleted := fs.resolveFeedTargets(context.Background(), viewer, feeds)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if !deleted.posts[1001] || !deleted.posts[1002] {
		t.Fatalf("query error must fail closed (all post targets inaccessible), got %+v", deleted.posts)
	}
	if !deleted.activities[3003] {
		t.Fatalf("query error must fail closed (all activity targets inaccessible), got %+v", deleted.activities)
	}
}

// TestFeedTargetDeleted 只有指向帖子/活动且该目标已不可展示时为 true；无目标（postID/actID=0）恒为 false。
func TestFeedTargetDeleted(t *testing.T) {
	deleted := feedDeletedTargets{
		posts:      map[int64]bool{1001: true},
		activities: map[int64]bool{3003: true},
	}

	if !feedTargetDeleted(feedTarget{postID: 1001}, deleted) {
		t.Fatalf("deleted post target should be true")
	}
	if feedTargetDeleted(feedTarget{postID: 1002}, deleted) {
		t.Fatalf("still-existing post target should be false")
	}
	if !feedTargetDeleted(feedTarget{actID: 3003}, deleted) {
		t.Fatalf("rejected activity target should be true")
	}
	if feedTargetDeleted(feedTarget{actID: 3004}, deleted) {
		t.Fatalf("normal activity target should be false")
	}
	if feedTargetDeleted(feedTarget{}, deleted) {
		t.Fatalf("target without post/activity should be false")
	}
}

// TestFeedMessageMarksDeletedPost 目标帖子已删时，feed 文案改为"帖子已不存在"。
func TestFeedMessageMarksDeletedPost(t *testing.T) {
	fs, _, _ := newFeedServiceForTest(t)
	f := &model.Feed{Action: "like", Object: SubjectPost, TargetId: 1001}

	got := fs.feedMessage(f, feedTarget{postID: 1001}, "张三", feedDeletedTargets{posts: map[int64]bool{1001: true}})
	if got != feedTargetDeletedMsg {
		t.Fatalf("expected %q, got %q", feedTargetDeletedMsg, got)
	}
}

// TestFeedMessageMarksRejectedActivity 活动被改判不通过时，feed 文案改为"活动已不存在"。
func TestFeedMessageMarksRejectedActivity(t *testing.T) {
	fs, _, _ := newFeedServiceForTest(t)
	f := &model.Feed{Action: "like", Object: SubjectActivity, TargetId: 3003}

	got := fs.feedMessage(f, feedTarget{actID: 3003}, "张三", feedDeletedTargets{activities: map[int64]bool{3003: true}})
	if got != feedActivityDeletedMsg {
		t.Fatalf("expected %q, got %q", feedActivityDeletedMsg, got)
	}
}

// TestFeedMessageKeepsMessageWhenPostExists 帖子仍在时保留原互动文案。
func TestFeedMessageKeepsMessageWhenPostExists(t *testing.T) {
	fs, _, _ := newFeedServiceForTest(t)
	f := &model.Feed{Action: "like", Object: SubjectPost, TargetId: 1001}

	got := fs.feedMessage(f, feedTarget{postID: 1001}, "张三", feedDeletedTargets{posts: map[int64]bool{1001: false}})
	if got != "张三赞了你的帖子" {
		t.Fatalf("existing post must keep the original message, got %q", got)
	}
}

// TestFeedMessageKeepsMessageWhenActivityNormal 活动仍在 pending/pass 时保留原互动文案。
func TestFeedMessageKeepsMessageWhenActivityNormal(t *testing.T) {
	fs, _, _ := newFeedServiceForTest(t)
	f := &model.Feed{Action: "collect", Object: SubjectActivity, TargetId: 3003}

	got := fs.feedMessage(f, feedTarget{actID: 3003}, "张三", feedDeletedTargets{activities: map[int64]bool{}})
	if got != "张三收藏了你的活动" {
		t.Fatalf("normal activity must keep the original message, got %q", got)
	}
}

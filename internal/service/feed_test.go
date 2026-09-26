package service

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/raiki02/EG/internal/model"
)

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

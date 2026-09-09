package service

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/mq"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// newCommentServiceForTest 构造 sqlmock DB + miniredis 的 CommentService，
// 覆盖 CreateComment/AnswerComment 的 root_id 继承逻辑。
func newCommentServiceForTest(t *testing.T) (*CommentService, sqlmock.Sqlmock) {
	t.Helper()

	// EncryptedString 写库需要 PII 密钥
	t.Setenv("EG_PII_KEY", "test-pii-key")

	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	ls := logger.NewLoggerSet()
	cd := dao.NewCommentDao(gdb, ls)
	ud := repo.NewUserRepo(dao.NewUserDao(gdb, ls), nil)
	lfr := cache.NewLikeFavoriteRedis(rdb)
	cfg := &config.Conf{}
	cfg.Auditor.Effect = "fast"
	ar := repo.NewActivityRepo(dao.NewActDao(gdb, cfg, ls), cache.NewCache(rdb))
	pr := repo.NewPostRepo(dao.NewPostDao(gdb, cfg, ls), cache.NewCache(rdb))
	ir := repo.NewInteractionRepo(dao.NewInteractionDao(gdb, ls), nil, ar, pr, lfr)
	sg := NewSubjectGetter(ar, pr, cd)
	testMQ := mq.NewMQ(rdb)
	svc := NewCommentService(cd, ud, ir, testMQ, sg, ls)
	return svc, mock
}

const (
	actID  int64 = 1001 // 活动 ID
	userA        = "S20250001" // A：一级评论作者
	userB        = "S20250002" // B：回复 A 的人
	userC        = "S20250003" // C：回复 B 的人
)

// mockActivityPreloads 预置 FindActById 的主查询与两个 Preload 查询（Images -> Signers）
func mockActivityPreloads(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT \\* FROM `activity`").
		WillReturnRows(sqlmock.NewRows([]string{"id", "student_id"}).AddRow(actID, userA))
	mock.ExpectQuery("SELECT \\* FROM `image`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT \\* FROM `activity_signer`").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
}

// mockGetUserInfo 预置评论者的用户信息查询（CreateComment/AnswerComment 第一步均会查评论者）
func mockGetUserInfo(mock sqlmock.Sqlmock, studentID string, name string) {
	mock.ExpectQuery("SELECT \\* FROM `user` WHERE student_id = \\?").
		WithArgs(studentID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "student_id", "name", "avatar"}).
			AddRow(1, studentID, name, ""))
}

// seedTopComment 预置一条一级评论 A（parent_id=活动ID，subject=activity），返回入库参数
func seedTopComment() model.Comment {
	return model.Comment{
		Id:             1,
		StudentID:      userA,
		Content:        "top",
		ParentID:       actID,
		RootID:         actID,
		Subject:        "activity",
		RootObjectID:   actID,
		RootObjectType: "activity",
		CreatorName:    "Alice",
	}
}

func TestAnswerCommentReplyToTopSetsRootToTop(t *testing.T) {
	cs, mock := newCommentServiceForTest(t)
	ctx := context.Background()
	mockGetUserInfo(mock, userB, "Bob")

	top := seedTopComment()
	rows := sqlmock.NewRows([]string{"id", "student_id", "parent_id", "root_id", "subject", "root_object_id", "root_object_type", "creator_name"}).
		AddRow(top.Id, top.StudentID, top.ParentID, top.RootID, top.Subject, top.RootObjectID, top.RootObjectType, string(top.CreatorName))
	mock.ExpectQuery("SELECT \\* FROM `comment` WHERE id = \\?").
		WithArgs(int64(1), sqlmock.AnyArg()).
		WillReturnRows(rows)
	mockActivityPreloads(mock)
	mock.ExpectExec("INSERT INTO `comment`").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectExec("UPDATE `activity` SET `comment_num`").WillReturnResult(sqlmock.NewResult(0, 1))

	// C 端参数：回复 A，subject 传了脏值验证强制覆盖
	cmt := &model.Comment{
		Id:        2,
		StudentID: userB,
		Content:   "reply to top",
		ParentID:  1,
		Subject:   "post",
	}
	got, err := cs.AnswerComment(ctx, cmt, userB)
	if err != nil {
		t.Fatal(err)
	}

	if got.Subject != SubjectComment {
		t.Errorf("Subject = %q, want %q", got.Subject, SubjectComment)
	}
	if got.RootID != int64(1) {
		t.Errorf("RootID = %d, want 1 (top comment id)", got.RootID)
	}
	// 回复一级评论（直接回复楼主）不带 ReplyTo，前端用 parentId==rootId 区分
	if got.ReplyToUserID != "" {
		t.Errorf("ReplyToUserID = %q, want empty (reply to top carries no ReplyTo)", got.ReplyToUserID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestAnswerCommentReplyToChildInheritsTopRootID(t *testing.T) {
	cs, mock := newCommentServiceForTest(t)
	ctx := context.Background()
	mockGetUserInfo(mock, userC, "Carol")

	rows := sqlmock.NewRows([]string{"id", "student_id", "parent_id", "root_id", "subject", "root_object_id", "root_object_type", "creator_name"}).
		AddRow(2, userB, 1, 1, "comment", actID, "activity", "Bob")
	mock.ExpectQuery("SELECT \\* FROM `comment` WHERE id = \\?").
		WithArgs(int64(2), sqlmock.AnyArg()).
		WillReturnRows(rows)
	mockActivityPreloads(mock)
	mock.ExpectExec("INSERT INTO `comment`").WillReturnResult(sqlmock.NewResult(3, 1))
	mock.ExpectExec("UPDATE `activity` SET `comment_num`").WillReturnResult(sqlmock.NewResult(0, 1))

	// C 回复 B（三级）
	cmt := &model.Comment{
		Id:        3,
		StudentID: userC,
		Content:   "reply to B",
		ParentID:  2,
		Subject:   SubjectComment,
	}
	got, err := cs.AnswerComment(ctx, cmt, userC)
	if err != nil {
		t.Fatal(err)
	}

	// 核心断言：三级评论 root_id 必须指向一级评论 A，而非直接父级 B
	if got.RootID != int64(1) {
		t.Errorf("RootID = %d, want 1 (top comment id, not parent's id)", got.RootID)
	}
	if got.RootObjectID != actID {
		t.Errorf("RootObjectID = %d, want %d", got.RootObjectID, actID)
	}
	if got.ReplyToUserID != userB {
		t.Errorf("ReplyToUserID = %q, want %q", got.ReplyToUserID, userB)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestCreateCommentSubjectCommentInheritsTopRootID(t *testing.T) {
	cs, mock := newCommentServiceForTest(t)
	ctx := context.Background()
	mockGetUserInfo(mock, userC, "Carol")

	rows := sqlmock.NewRows([]string{"id", "student_id", "parent_id", "root_id", "subject", "root_object_id", "root_object_type", "creator_name"}).
		AddRow(2, userB, 1, 1, "comment", actID, "activity", "Bob")
	mock.ExpectQuery("SELECT \\* FROM `comment` WHERE id = \\?").
		WithArgs(int64(2), sqlmock.AnyArg()).
		WillReturnRows(rows)
	mock.ExpectQuery("SELECT \\* FROM `comment` WHERE id = \\?").
		WithArgs(int64(2), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "student_id"}).AddRow(2, userB))
	mockActivityPreloads(mock)
	mock.ExpectExec("INSERT INTO `comment`").WillReturnResult(sqlmock.NewResult(4, 1))
	mock.ExpectExec("UPDATE `activity` SET `comment_num`").WillReturnResult(sqlmock.NewResult(0, 1))

	cmt := &model.Comment{
		Id:        4,
		StudentID: userC,
		Content:   "create path reply to B",
		ParentID:  2,
		Subject:   SubjectComment,
	}
	got, err := cs.CreateComment(ctx, cmt, userC)
	if err != nil {
		t.Fatal(err)
	}

	if got.RootID != int64(1) {
		t.Errorf("RootID = %d, want 1 (top comment id, not parent's id)", got.RootID)
	}
	if got.ReplyToUserID != userB {
		t.Errorf("ReplyToUserID = %q, want %q", got.ReplyToUserID, userB)
	}
	if got.ReplyToUserName != "Bob" {
		t.Errorf("ReplyToUserName = %q, want %q", got.ReplyToUserName, "Bob")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestAnswerCommentRootNotFoundRejectsBeforeInsert(t *testing.T) {
	cs, mock := newCommentServiceForTest(t)
	ctx := context.Background()
	mockGetUserInfo(mock, userC, "Carol")

	rows := sqlmock.NewRows([]string{"id", "student_id", "parent_id", "root_id", "subject", "root_object_id", "root_object_type", "creator_name"}).
		AddRow(2, userB, 1, 1, "comment", actID, "activity", "Bob")
	mock.ExpectQuery("SELECT \\* FROM `comment` WHERE id = \\?").
		WithArgs(int64(2), sqlmock.AnyArg()).
		WillReturnRows(rows)
	// 根对象校验失败：返回 ErrRecordNotFound
	mock.ExpectQuery("SELECT \\* FROM `activity`").WillReturnError(gorm.ErrRecordNotFound)

	cmt := &model.Comment{
		Id:        5,
		StudentID: userC,
		Content:   "orphan reply",
		ParentID:  2,
		Subject:   SubjectComment,
	}
	_, err := cs.AnswerComment(ctx, cmt, userC)
	if err == nil {
		t.Fatal("want error when root object missing")
	}

	// 核心断言：校验失败时不得有任何 INSERT 发生
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected SQL expectations: %v", err)
	}
}

// 删除一级评论：级联删子级 + 活动计数回减
func TestDeleteTopCommentCascadeAndDecreaseNum(t *testing.T) {
	cs, mock := newCommentServiceForTest(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT `id`,`student_id`,`parent_id`,`root_id`,`subject`,`root_object_id`,`root_object_type` FROM `comment`").
		WithArgs(int64(1), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "student_id", "parent_id", "root_id", "subject", "root_object_id", "root_object_type"}).
			AddRow(1, userA, actID, actID, "activity", actID, "activity"))
	// 级联删除事务：本人评论 + root 子树（2 条：B、C）
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `comment` WHERE student_id = \\? and id = \\?").
		WithArgs(userA, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `comment` WHERE root_id = \\? AND subject = 'comment'").
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()
	// 计数回减 3（一级 + B + C），CASE WHEN 表达式的 n 出现两次 + WHERE id
	mock.ExpectExec("UPDATE `activity` SET `comment_num`").
		WithArgs(int64(3), int64(3), actID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := cs.DeleteComment(ctx, 1, userA)
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// 删除子级评论：只删自身（孙级回复挂一级继续展示），回减 1
func TestDeleteChildCommentOnlyDeletesSelf(t *testing.T) {
	cs, mock := newCommentServiceForTest(t)
	ctx := context.Background()

	// 查归属：B 是二级（subject=comment，root_object=activity）
	mock.ExpectQuery("SELECT `id`,`student_id`,`parent_id`,`root_id`,`subject`,`root_object_id`,`root_object_type` FROM `comment`").
		WithArgs(int64(2), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "student_id", "parent_id", "root_id", "subject", "root_object_id", "root_object_type"}).
			AddRow(2, userB, 1, 1, "comment", actID, "activity"))
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `comment` WHERE student_id = \\? and id = \\?").
		WithArgs(userB, int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// B 是子级：root_id=2 的子树不存在（孙级 root_id 指向一级 A），命中 0 行
	mock.ExpectExec("DELETE FROM `comment` WHERE root_id = \\? AND subject = 'comment'").
		WithArgs(int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	// 只删 B 一条 = 回减 1，CASE WHEN 表达式的 n 出现两次 + WHERE id
	mock.ExpectExec("UPDATE `activity` SET `comment_num`").
		WithArgs(int64(1), int64(1), actID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := cs.DeleteComment(ctx, 2, userB)
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// 删除他人评论：静默 no-op 返回成功，与"不存在"表现一致，且不发任何 DELETE
func TestDeleteOthersCommentIgnored(t *testing.T) {
	cs, mock := newCommentServiceForTest(t)
	ctx := context.Background()

	// 评论存在但作者是 A，操作者是 C：service 层越权校验即拦截，不发 DELETE
	mock.ExpectQuery("SELECT `id`,`student_id`,`parent_id`,`root_id`,`subject`,`root_object_id`,`root_object_type` FROM `comment`").
		WithArgs(int64(1), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "student_id", "parent_id", "root_id", "subject", "root_object_id", "root_object_type"}).
			AddRow(1, userA, actID, actID, "activity", actID, "activity"))

	if err := cs.DeleteComment(ctx, 1, userC); err != nil {
		t.Fatalf("want nil (silent no-op) when deleting others' comment, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// 删除不存在的评论：幂等成功（不视为错误），无任何 DELETE
func TestDeleteNonExistentCommentIdempotent(t *testing.T) {
	cs, mock := newCommentServiceForTest(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT `id`,`student_id`,`parent_id`,`root_id`,`subject`,`root_object_id`,`root_object_type` FROM `comment`").
		WithArgs(int64(999), sqlmock.AnyArg()).
		WillReturnError(gorm.ErrRecordNotFound)

	err := cs.DeleteComment(ctx, 999, userC)
	if err != nil {
		t.Fatalf("want nil when deleting non-existent comment, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// 一级评论自评（评论自己的活动）：也要计数 +1（修复前 return 在计数前）
func TestCreateSelfCommentStillCounts(t *testing.T) {
	cs, mock := newCommentServiceForTest(t)
	ctx := context.Background()

	// A 评论自己的活动（subject=activity，parent=actID，作者=A）
	mockGetUserInfo(mock, userA, "Alice")
	mockActivityPreloads(mock)
	mock.ExpectExec("INSERT INTO `comment`").WillReturnResult(sqlmock.NewResult(10, 1))
	// 计数 +1（自评也要计）
	mock.ExpectExec("UPDATE `activity` SET `comment_num`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 自评不进 Feed，无 Publish

	cmt := &model.Comment{
		Id:        10,
		StudentID: userA,
		Content:   "self comment",
		ParentID:  actID,
		Subject:   SubjectActivity,
	}
	_, err := cs.CreateComment(ctx, cmt, userA)
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
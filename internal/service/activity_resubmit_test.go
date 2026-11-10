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
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func newResubmitServiceForTest(t *testing.T, maxResubmit int) (*ActivityService, sqlmock.Sqlmock) {
	t.Helper()
	t.Setenv("EG_PII_KEY", "0123456789abcdef0123456789abcdef")

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

	actDao := dao.NewActDao(gdb, &config.Conf{}, logger.NewLoggerSet())
	svc := &ActivityService{
		ad:          repo.NewActivityRepo(actDao, nil),
		mq:          &fakeFeedMQ{},
		maxResubmit: maxResubmit,
		l:           zap.NewNop(),
	}
	return svc, mock
}

// expectResubmitSourceLoad 模拟 findById 读取源活动及其 Signers/Images 预加载。
func expectResubmitSourceLoad(mock sqlmock.Sqlmock, srcID int64, revision int, replacedBy int64) {
	rows := sqlmock.NewRows([]string{
		"id", "student_id", "is_checking", "revision", "replaced_by", "title", "start_time", "end_time",
	}).AddRow(srcID, "S20250001", "reject", revision, replacedBy, "源活动标题", "2026-01-01 10:00:00", "2026-01-01 12:00:00")

	mock.ExpectQuery("SELECT \\* FROM `activity` WHERE id = \\? ORDER BY `activity`.`id` LIMIT \\?").
		WithArgs(srcID, 1).
		WillReturnRows(rows)
	mock.ExpectQuery("SELECT \\* FROM `image`").WillReturnRows(sqlmock.NewRows([]string{"owner_id"}))
	mock.ExpectQuery("SELECT \\* FROM `activity_signer`").WillReturnRows(sqlmock.NewRows([]string{"activity_id"}))
}

// expectResubmitSourceState 模拟并发冲突后重读源活动：is_checking 与 replaced_by 决定错误码。
func expectResubmitSourceState(mock sqlmock.Sqlmock, srcID int64, isChecking string, replacedBy int64) {
	rows := sqlmock.NewRows([]string{
		"id", "student_id", "is_checking", "revision", "replaced_by", "title", "start_time", "end_time",
	}).AddRow(srcID, "S20250001", isChecking, 0, replacedBy, "源活动标题", "2026-01-01 10:00:00", "2026-01-01 12:00:00")

	mock.ExpectQuery("SELECT \\* FROM `activity` WHERE id = \\? ORDER BY `activity`.`id` LIMIT \\?").
		WithArgs(srcID, 1).
		WillReturnRows(rows)
	mock.ExpectQuery("SELECT \\* FROM `image`").WillReturnRows(sqlmock.NewRows([]string{"owner_id"}))
	mock.ExpectQuery("SELECT \\* FROM `activity_signer`").WillReturnRows(sqlmock.NewRows([]string{"activity_id"}))
}

// expectResubmitCreate 模拟回写源活动 replaced_by 并创建新活动的完整事务。
// 重新送审保留用户正在编辑的活动草稿，因此不应出现 activity_draft 的查询与删除。
func expectResubmitCreate(mock sqlmock.Sqlmock, newID, srcID int64, withSigners bool, replacedRows int64) {
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `activity` SET `replaced_by`=\\? WHERE id = \\? AND replaced_by = 0 AND is_checking = \\?").
		WithArgs(newID, srcID, "reject").
		WillReturnResult(sqlmock.NewResult(0, replacedRows))
	if replacedRows == 0 {
		mock.ExpectRollback()
		return
	}
	mock.ExpectExec("INSERT INTO `activity`").WillReturnResult(sqlmock.NewResult(0, 1))
	if withSigners {
		mock.ExpectExec("INSERT INTO `activity_signer`").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("INSERT INTO `approvement`").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()
}

func newResubmitAct() *model.Activity {
	return &model.Activity{
		Id:             tools.MustGenerateID(),
		StudentID:      "S20250001",
		Title:          "源活动标题",
		Introduce:      "编辑后的介绍",
		HolderType:     "个人",
		OrganizerUnit:  "计算机学院",
		Position:       "南湖校区",
		Address:        "教学楼 101",
		IfRegister:     "是",
		RegisterMethod: "线上",
		StartTime:      "2026-01-01 10:00:00",
		EndTime:        "2026-01-01 12:00:00",
		Type:           "讲座",
		ActiveForm:     "https://example.com/form.pdf",
	}
}

// TestResubmitActivityCreatesNewRevision 内容与源活动完全一致也能创建成功（唯一索引含 revision），
// 新活动 revision 为源 revision+1，源活动的 replaced_by 被指向新活动。
func TestResubmitActivityCreatesNewRevision(t *testing.T) {
	svc, mock := newResubmitServiceForTest(t, 5)
	srcID := int64(10001)
	act := newResubmitAct()
	signers := []model.Signer{{StudentID: "S20250002", Name: "张三"}}

	expectResubmitSourceLoad(mock, srcID, 1, 0)
	expectResubmitCreate(mock, act.Id, srcID, true, 1)

	if err := svc.ResubmitActivity(context.Background(), srcID, act, signers, "S20250001"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act.Revision != 2 {
		t.Fatalf("revision = %d, want 2", act.Revision)
	}
	if act.Id == srcID {
		t.Fatal("new activity must have a new id")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestResubmitActivityWithoutSignersKeepsPendingAuditor 无签署人时仍沿用创建逻辑直接置为待审核。
func TestResubmitActivityWithoutSignersKeepsPendingAuditor(t *testing.T) {
	svc, mock := newResubmitServiceForTest(t, 5)
	srcID := int64(10002)
	act := newResubmitAct()

	expectResubmitSourceLoad(mock, srcID, 0, 0)
	expectResubmitCreate(mock, act.Id, srcID, false, 1)

	if err := svc.ResubmitActivity(context.Background(), srcID, act, nil, "S20250001"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act.Revision != 1 {
		t.Fatalf("revision = %d, want 1", act.Revision)
	}
	if act.IsChecking != "pending_auditor" {
		t.Fatalf("is_checking = %q, want pending_auditor", act.IsChecking)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestResubmitActivityRollbackWhenAlreadyReplaced 并发下源活动已被替代时回滚，不留下新活动。
func TestResubmitActivityRollbackWhenAlreadyReplaced(t *testing.T) {
	svc, mock := newResubmitServiceForTest(t, 5)
	srcID := int64(10003)
	act := newResubmitAct()

	expectResubmitSourceLoad(mock, srcID, 1, 0)
	expectResubmitCreate(mock, act.Id, srcID, false, 0)
	// 条件更新未命中后重读源活动：已被指向新申请。
	expectResubmitSourceState(mock, srcID, "reject", 10004)

	err := svc.ResubmitActivity(context.Background(), srcID, act, nil, "S20250001")
	if !errors.Is(err, errs.ErrActivityAlreadyReplaced) {
		t.Fatalf("err = %v, want ErrActivityAlreadyReplaced", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestResubmitActivityRollbackWhenRejectedVerdictChanged 读取源活动后被平台改判为通过：
// 条件更新因 is_checking 不再是 reject 而未命中，返回"未驳回"而不是"已替代"。
func TestResubmitActivityRollbackWhenRejectedVerdictChanged(t *testing.T) {
	svc, mock := newResubmitServiceForTest(t, 5)
	srcID := int64(10005)
	act := newResubmitAct()

	expectResubmitSourceLoad(mock, srcID, 1, 0)
	expectResubmitCreate(mock, act.Id, srcID, false, 0)
	expectResubmitSourceState(mock, srcID, model.StancePass, 0)

	err := svc.ResubmitActivity(context.Background(), srcID, act, nil, "S20250001")
	if !errors.Is(err, errs.ErrActivityNotRejected) {
		t.Fatalf("err = %v, want ErrActivityNotRejected", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestValidateResubmit 锁定重新送审的前置条件与次数上限。
func TestValidateResubmit(t *testing.T) {
	const maxResubmit = 5
	base := model.Activity{
		Id:         1,
		StudentID:  "S20250001",
		IsChecking: model.StanceReject,
		Revision:   0,
		ReplacedBy: 0,
	}

	cases := []struct {
		name    string
		mutate  func(*model.Activity)
		viewer  string
		wantErr error
	}{
		{name: "创建者被驳回可送审", mutate: func(*model.Activity) {}, viewer: "S20250001"},
		{name: "非创建者不可送审", mutate: func(*model.Activity) {}, viewer: "S20250002", wantErr: errs.ErrActivityNotOwner},
		{name: "源活动未驳回", mutate: func(a *model.Activity) { a.IsChecking = "pending_auditor" }, viewer: "S20250001", wantErr: errs.ErrActivityNotRejected},
		{name: "源活动已被替代", mutate: func(a *model.Activity) { a.ReplacedBy = 99 }, viewer: "S20250001", wantErr: errs.ErrActivityAlreadyReplaced},
		{name: "达到次数上限", mutate: func(a *model.Activity) { a.Revision = maxResubmit }, viewer: "S20250001", wantErr: errs.ErrActivityResubmitLimit},
		{name: "未达上限", mutate: func(a *model.Activity) { a.Revision = maxResubmit - 1 }, viewer: "S20250001"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := base
			tc.mutate(&src)
			err := validateResubmit(src, tc.viewer, maxResubmit)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

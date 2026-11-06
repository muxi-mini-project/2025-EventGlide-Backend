package service

import (
	"context"
	"testing"

	"github.com/raiki02/EG/internal/model"
	"go.uber.org/zap"
)

// TestRejectReasonsOnlyForRejectedActs 只有审核不通过的活动才透出原因；
// 平台在 pass 时也可能带 msg，不能当作拒绝原因展示。
func TestRejectReasonsOnlyForRejectedActs(t *testing.T) {
	aud := &fakeAuditorSvc{reasons: map[int64]string{1: "资料不全", 2: "通过时也带的原因"}}
	svc := &ActivityService{aud: aud, l: zap.NewNop()}

	got := svc.RejectReasons(context.Background(),
		model.Activity{Id: 1, IsChecking: "reject"},
		model.Activity{Id: 2, IsChecking: "pass"},
	)

	if got[1] != "资料不全" {
		t.Fatalf("expected reason for rejected activity, got %+v", got)
	}
	if _, ok := got[2]; ok {
		t.Fatalf("pass activity must not carry rejectReason, got %+v", got)
	}
	if len(aud.reasonIds) != 1 || aud.reasonIds[0] != 1 {
		t.Fatalf("only rejected activity should be queried, got %v", aud.reasonIds)
	}
}

// TestRejectReasonsNoRejectedActs 没有 reject 活动时不查库。
func TestRejectReasonsNoRejectedActs(t *testing.T) {
	aud := &fakeAuditorSvc{}
	svc := &ActivityService{aud: aud, l: zap.NewNop()}

	if got := svc.RejectReasons(context.Background(), model.Activity{Id: 1, IsChecking: "pending_auditor"}); got != nil {
		t.Fatalf("expected nil reasons, got %+v", got)
	}
	if len(aud.reasonIds) != 0 {
		t.Fatalf("no rejected activity, expected no query, got ids %v", aud.reasonIds)
	}
}

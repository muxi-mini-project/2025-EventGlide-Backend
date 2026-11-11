package handler

import (
	"testing"

	"github.com/raiki02/EG/internal/model"
)

// TestDetailFieldsVisible 签署人、申请表等编辑详情只对创建者与签署人可见。
func TestDetailFieldsVisible(t *testing.T) {
	act := model.Activity{StudentID: "S20250001"}

	cases := []struct {
		name     string
		viewer   string
		isSigner bool
		want     bool
	}{
		{name: "创建者可见", viewer: "S20250001", want: true},
		{name: "签署人可见", viewer: "S20250002", isSigner: true, want: true},
		{name: "其他人不可见", viewer: "S20250003", want: false},
		{name: "空 viewer 不可见", viewer: "", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detailFieldsVisible(act, tc.viewer, tc.isSigner); got != tc.want {
				t.Fatalf("detailFieldsVisible(viewer=%q, isSigner=%v) = %v, want %v",
					tc.viewer, tc.isSigner, got, tc.want)
			}
		})
	}
}

// TestActivityVisibleToViewer 被驳回（下架）的活动只对创建者与签署人可见，其他人按"不存在"处理。
func TestActivityVisibleToViewer(t *testing.T) {
	cases := []struct {
		name     string
		status   string
		viewer   string
		isSigner bool
		want     bool
	}{
		{name: "已通过对所有人可见", status: model.StancePass, viewer: "S20250003", want: true},
		{name: "审核中对所有人可见", status: "pending_auditor", viewer: "S20250003", want: true},
		{name: "被驳回时创建者可见", status: model.StanceReject, viewer: "S20250001", want: true},
		{name: "被驳回时签署人可见", status: model.StanceReject, viewer: "S20250002", isSigner: true, want: true},
		{name: "被驳回时他人不可见", status: model.StanceReject, viewer: "S20250002", want: false},
		{name: "被驳回时空 viewer 不可见", status: model.StanceReject, viewer: "", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			act := model.Activity{StudentID: "S20250001", IsChecking: tc.status}
			if got := activityVisibleToViewer(act, tc.viewer, tc.isSigner); got != tc.want {
				t.Fatalf("activityVisibleToViewer(status=%q, viewer=%q, isSigner=%v) = %v, want %v",
					tc.status, tc.viewer, tc.isSigner, got, tc.want)
			}
		})
	}
}

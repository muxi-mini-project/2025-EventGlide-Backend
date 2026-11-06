package handler

import (
	"testing"

	"github.com/raiki02/EG/internal/model"
)

// TestRejectReasonVisible 拒绝原因只对活动作者本人可见。
func TestRejectReasonVisible(t *testing.T) {
	act := model.Activity{StudentID: "S20250001"}

	cases := []struct {
		name   string
		viewer string
		want   bool
	}{
		{name: "作者本人可见", viewer: "S20250001", want: true},
		{name: "他人不可见", viewer: "S20250002", want: false},
		{name: "空 viewer 不可见", viewer: "", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rejectReasonVisible(act, tc.viewer); got != tc.want {
				t.Fatalf("rejectReasonVisible(viewer=%q) = %v, want %v", tc.viewer, got, tc.want)
			}
		})
	}
}

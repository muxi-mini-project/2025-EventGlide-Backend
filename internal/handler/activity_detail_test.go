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

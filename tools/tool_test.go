package tools

import "testing"

// TestStatusMapper 锁定回调状态映射契约：平台人工审核发中文、AI 审核发英文，
// 两种都要识别；未知值返回空串，由调用方拒绝而非写入非法枚举。
func TestStatusMapper(t *testing.T) {
	cases := map[string]string{
		"未审核":           "pending",
		"通过":            "pass",
		"不通过":           "reject",
		"Pending":       "pending",
		"Pass":          "pass",
		"Reject":        "reject",
		"pass":          "pass",
		"PASS":          "pass",
		"":              "",
		"unknown":       "",
		"UnknownStatus": "",
	}
	for in, want := range cases {
		if got := StatusMapper(in); got != want {
			t.Errorf("StatusMapper(%q) = %q, want %q", in, got, want)
		}
	}
}

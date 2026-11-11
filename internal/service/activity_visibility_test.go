package service

import (
	"testing"

	"github.com/raiki02/EG/internal/model"
)

// TestActivityVisible 活动可见性的唯一规则：创建者与签署人始终可见；
// 普通访客按 effect 分流（slow 只放 pass，fast 放除 reject 外的状态），
// effect 未配置或非法一律按 slow fail-closed。
// 期望值逐行钉死，不用实现自身的推导充当断言。
func TestActivityVisible(t *testing.T) {
	const owner = "S20250001"
	const signer = "S20250002"
	const stranger = "S20250003"

	cases := []struct {
		effect   string
		status   string
		owner    bool
		signer   bool
		stranger bool
		anon     bool
	}{
		{"slow", model.StancePass, true, true, true, true},
		{"slow", "pending_auditor", true, true, false, false},
		{"slow", "pending_signers", true, true, false, false},
		{"slow", model.StanceReject, true, true, false, false},

		{"fast", model.StancePass, true, true, true, true},
		{"fast", "pending_auditor", true, true, true, true},
		{"fast", "pending_signers", true, true, true, true},
		{"fast", model.StanceReject, true, true, false, false},
		// 历史空值/NULL 不因 fast 而放行：只有明确处于审核中或已通过才算可见。
		{"fast", "", true, true, false, false},

		// 未配置：等同 slow。
		{"", model.StancePass, true, true, true, true},
		{"", "pending_auditor", true, true, false, false},
		{"", "pending_signers", true, true, false, false},
		{"", model.StanceReject, true, true, false, false},

		// 非法值：同样按 slow 处理。
		{"unknown", model.StancePass, true, true, true, true},
		{"unknown", "pending_auditor", true, true, false, false},
		{"unknown", "pending_signers", true, true, false, false},
		{"unknown", model.StanceReject, true, true, false, false},
	}

	for _, tc := range cases {
		viewers := []struct {
			name     string
			viewer   string
			isSigner bool
			want     bool
		}{
			{"创建者", owner, false, tc.owner},
			{"签署人", signer, true, tc.signer},
			{"普通访客", stranger, false, tc.stranger},
			{"未登录", "", false, tc.anon},
		}

		for _, v := range viewers {
			name := tc.effect + "/" + tc.status + "/" + v.name
			if tc.effect == "" {
				name = "unset/" + tc.status + "/" + v.name
			}
			t.Run(name, func(t *testing.T) {
				got := ActivityVisible(tc.status, owner, v.viewer, v.isSigner, tc.effect)
				if got != v.want {
					t.Fatalf("ActivityVisible(isChecking=%q, viewer=%q, isSigner=%v, effect=%q) = %v, want %v",
						tc.status, v.viewer, v.isSigner, tc.effect, got, v.want)
				}
			})
		}
	}
}

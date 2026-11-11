package service

import (
	"testing"

	"github.com/raiki02/EG/api/req"
)

// TestNormalizeActScope scope 归一化：空值与非法值（含大小写不符）一律按 active，
// 只有精确匹配才生效，保证不传新参数的老客户端行为不变。
func TestNormalizeActScope(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "空值默认 active", in: "", want: req.ActScopeActive},
		{name: "active", in: req.ActScopeActive, want: req.ActScopeActive},
		{name: "ended", in: req.ActScopeEnded, want: req.ActScopeEnded},
		{name: "all", in: req.ActScopeAll, want: req.ActScopeAll},
		{name: "非法值", in: "unknown", want: req.ActScopeActive},
		{name: "大小写不符按非法", in: "ACTIVE", want: req.ActScopeActive},
		{name: "大小写混杂按非法", in: "Ended", want: req.ActScopeActive},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeActScope(tc.in); got != tc.want {
				t.Fatalf("normalizeActScope(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestNormalizeActRole role 归一化：空值与非法值（含大小写不符）一律按 creator。
func TestNormalizeActRole(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "空值默认 creator", in: "", want: req.ActRoleCreator},
		{name: "creator", in: req.ActRoleCreator, want: req.ActRoleCreator},
		{name: "participant", in: req.ActRoleParticipant, want: req.ActRoleParticipant},
		{name: "all", in: req.ActRoleAll, want: req.ActRoleAll},
		{name: "非法值", in: "owner", want: req.ActRoleCreator},
		{name: "大小写不符按非法", in: "PARTICIPANT", want: req.ActRoleCreator},
		{name: "大小写混杂按非法", in: "Creator", want: req.ActRoleCreator},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeActRole(tc.in); got != tc.want {
				t.Fatalf("normalizeActRole(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

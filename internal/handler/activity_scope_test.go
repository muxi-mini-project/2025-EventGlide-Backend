package handler

import "testing"

// TestShouldUseOwnActivityList 只有路径学号是本人、且显式传了 scope 或 role
// （显式默认值也算传参）时才切到本人全状态口径；空白字符串按未传处理。
func TestShouldUseOwnActivityList(t *testing.T) {
	const me = "S20250001"
	const other = "S20250002"

	cases := []struct {
		name          string
		claimsSubject string
		pathStudentID string
		scope         string
		role          string
		want          bool
	}{
		{name: "本人传 scope=active", claimsSubject: me, pathStudentID: me, scope: "active", want: true},
		{name: "本人传 role=creator", claimsSubject: me, pathStudentID: me, role: "creator", want: true},
		{name: "本人两个都传", claimsSubject: me, pathStudentID: me, scope: "ended", role: "participant", want: true},
		{name: "本人两个都空", claimsSubject: me, pathStudentID: me, want: false},
		{name: "本人 scope 空白", claimsSubject: me, pathStudentID: me, scope: "   ", want: false},
		{name: "本人 role 空白", claimsSubject: me, pathStudentID: me, role: "  ", want: false},
		{name: "看别人且传参数", claimsSubject: me, pathStudentID: other, scope: "active", role: "creator", want: false},
		{name: "看别人且不传参数", claimsSubject: me, pathStudentID: other, want: false},
		{name: "claims 与路径学号都为空", claimsSubject: "", pathStudentID: "", want: false},
		{name: "claims 有值但路径学号为空", claimsSubject: me, pathStudentID: "", scope: "active", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldUseOwnActivityList(tc.claimsSubject, tc.pathStudentID, tc.scope, tc.role); got != tc.want {
				t.Fatalf("shouldUseOwnActivityList(claims=%q, path=%q, scope=%q, role=%q) = %v, want %v",
					tc.claimsSubject, tc.pathStudentID, tc.scope, tc.role, got, tc.want)
			}
		})
	}
}

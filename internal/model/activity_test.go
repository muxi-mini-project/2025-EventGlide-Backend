package model

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

// TestActivityUniqueIndexIncludesRevision 活动唯一索引必须包含 revision，
// 否则被驳回后原样重新送审会撞 (student_id, title, start_time, end_time) 唯一键。
// revision 必须排在末位：activity 上没有其它以 student_id 为前缀的索引，
// 让它占首位会使个人主页、/act/own、GetChecking 等按 student_id 的查询失去索引前缀。
func TestActivityUniqueIndexIncludesRevision(t *testing.T) {
	s, err := schema.Parse(&Activity{}, &sync.Map{}, schema.NamingStrategy{SingularTable: true})
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}

	var target *schema.Index
	for _, idx := range s.ParseIndexes() {
		if idx.Name == "idx_activity_unique" {
			target = idx
			break
		}
	}
	if target == nil {
		t.Fatal("idx_activity_unique not found")
	}
	if target.Class != "UNIQUE" {
		t.Fatalf("idx_activity_unique class = %q, want UNIQUE", target.Class)
	}

	cols := make([]string, 0, len(target.Fields))
	seen := make(map[string]bool, len(target.Fields))
	for _, f := range target.Fields {
		cols = append(cols, f.DBName)
		seen[f.DBName] = true
	}
	for _, want := range []string{"student_id", "title", "start_time", "end_time", "revision"} {
		if !seen[want] {
			t.Fatalf("idx_activity_unique missing column %q, got %v", want, cols)
		}
	}

	wantOrder := []string{"student_id", "title", "start_time", "end_time", "revision"}
	if len(cols) != len(wantOrder) {
		t.Fatalf("idx_activity_unique columns = %v, want %v", cols, wantOrder)
	}
	for i := range wantOrder {
		if cols[i] != wantOrder[i] {
			t.Fatalf("idx_activity_unique column order = %v, want %v", cols, wantOrder)
		}
	}
}

package model

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

// TestActivityUniqueIndexIncludesRevision 活动唯一索引必须包含 revision，
// 否则被驳回后原样重新送审会撞 (student_id, title, start_time, end_time) 唯一键。
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

	cols := make(map[string]bool, len(target.Fields))
	for _, f := range target.Fields {
		cols[f.DBName] = true
	}
	for _, want := range []string{"student_id", "title", "start_time", "end_time", "revision"} {
		if !cols[want] {
			t.Fatalf("idx_activity_unique missing column %q, got %v", want, cols)
		}
	}
	if len(cols) != 5 {
		t.Fatalf("idx_activity_unique has %d columns, want 5: %v", len(cols), cols)
	}
}

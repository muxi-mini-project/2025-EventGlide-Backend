package mq

import (
	"errors"
	"fmt"
	"testing"

	"github.com/raiki02/EG/internal/dao"
	"gorm.io/gorm"
)

func TestIsNonRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"duplicate key", gorm.ErrDuplicatedKey, true},
		{"invalid subject", dao.ErrInvalidSubject, true},
		{"unknown type", ErrUnknownType, true},
		{"unknown like action", ErrUnknownLikeAction, true},
		{"unknown collect action", ErrUnknownCollectAction, true},
		{"wrapped unknown type", fmt.Errorf("%w: collect", ErrUnknownType), true},
		{"wrapped invalid subject", fmt.Errorf("insert: %w", dao.ErrInvalidSubject), true},
		{"db connection error", errors.New("sql: database is closed"), false},
		{"mysql invalid connection must NOT match", errors.New("invalid connection"), false},
		{"mysql driver error wrapper", fmt.Errorf("driver: bad connection"), false},
		{"timeout", errors.New("i/o timeout"), false},
	}
	for _, tt := range tests {
		if got := isNonRetryable(tt.err); got != tt.want {
			t.Errorf("%s: isNonRetryable = %v, want %v", tt.name, got, tt.want)
		}
	}
}
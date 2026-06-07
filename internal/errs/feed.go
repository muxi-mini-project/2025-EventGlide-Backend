package errs

import "github.com/raiki02/EG/pkg/errorx"

var (
	ErrFeedListFailed = errorx.New(FeedListFailed, "获取动态列表失败")
)
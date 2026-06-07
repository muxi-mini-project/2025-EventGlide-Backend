package errs

import "github.com/raiki02/EG/pkg/errorx"

var (
	ErrActivityNotFound     = errorx.New(ActivityNotFound, "活动不存在")
	ErrActivityExpired      = errorx.New(ActivityExpired, "活动已结束")
	ErrActivityCreateFailed = errorx.New(ActivityCreateFailed, "活动创建失败")
	ErrDraftNotFound        = errorx.New(DraftNotFound, "草稿不存在")
)
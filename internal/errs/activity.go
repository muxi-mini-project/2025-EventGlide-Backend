package errs

import "github.com/raiki02/EG/pkg/errorx"

var (
	ErrActivityNotFound     = errorx.New(ActivityNotFound, "活动不存在")
	ErrActivityExpired      = errorx.New(ActivityExpired, "活动已结束")
	ErrActivityCreateFailed = errorx.New(ActivityCreateFailed, "活动创建失败")
	ErrDraftNotFound        = errorx.New(DraftNotFound, "草稿不存在")

	ErrActivityResubmitLimit   = errorx.New(ActivityResubmitLimit, "重新送审次数已达上限")
	ErrActivityNotRejected     = errorx.New(ActivityNotRejected, "只有被驳回的活动可以重新送审")
	ErrActivityAlreadyReplaced = errorx.New(ActivityAlreadyReplaced, "该活动已被重新送审")
	ErrActivityNotOwner        = errorx.New(ActivityNotOwner, "只有创建者可以重新送审")
)

package errs

import "github.com/raiki02/EG/pkg/errorx"

var (
	ErrInteractionSubjectInvalid = errorx.New(InteractionSubjectInvalid, "无效的主题类型")
	ErrInteractionNotAllowed     = errorx.New(InteractionNotAllowed, "当前活动状态不允许该操作")
)

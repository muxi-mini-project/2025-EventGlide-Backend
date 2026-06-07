package errs

import "github.com/raiki02/EG/pkg/errorx"

var (
	ErrInteractionSubjectInvalid = errorx.New(InteractionSubjectInvalid, "无效的主题类型")
)
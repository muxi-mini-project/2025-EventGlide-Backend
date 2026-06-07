package errs

import "github.com/raiki02/EG/pkg/errorx"

var (
	ErrPostNotFound     = errorx.New(PostNotFound, "帖子不存在")
	ErrPostCreateFailed = errorx.New(PostCreateFailed, "帖子创建失败")
)
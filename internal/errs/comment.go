package errs

import "github.com/raiki02/EG/pkg/errorx"

var (
	ErrCommentNotFound       = errorx.New(CommentNotFound, "评论不存在")
	ErrCommentParentNotFound = errorx.New(CommentParentNotFound, "评论父级不存在")
	ErrInvalidSubject        = errorx.New(InvalidSubject, "无效的主题类型")
)
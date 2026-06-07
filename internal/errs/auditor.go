package errs

import "github.com/raiki02/EG/pkg/errorx"

var (
	ErrUploadFormFailed = errorx.New(UploadFormFailed, "表单上传失败")
	ErrCreateFormFailed = errorx.New(CreateFormFailed, "表单创建失败")
)
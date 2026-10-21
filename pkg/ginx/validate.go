package ginx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/pkg/errorx"
)

const (
	HolderTypePersonal = "个人"

	// maxReportedFields 限制返回给客户端的字段数量，避免一次列出几十个字段
	maxReportedFields = 3
)

var validate = validator.New()

func GetValidate() *validator.Validate {
	return validate
}

func validateRequest(any any) error {
	return validate.Struct(any)
}

// InitValidation 注册自定义校验与字段名映射，可重复调用
func InitValidation() {
	validate.RegisterTagNameFunc(jsonFieldName)

	validate.RegisterStructValidation(func(sl validator.StructLevel) {
		req_ := sl.Current().Interface().(req.CreateActReq)
		if req_.LabelForm.HolderType == HolderTypePersonal {
			if len(req_.LabelForm.Signer) <= 2 {
				sl.ReportError(req_.LabelForm.Signer, "signer", "Signer", "min_signer", "3")
				return
			}
		} else {
			if len(req_.LabelForm.Signer) <= 0 {
				sl.ReportError(req_.LabelForm.Signer, "signer", "Signer", "min_signer", "1")
				return
			}
		}
	}, req.CreateActReq{})
}

// jsonFieldName 让校验错误里的字段名使用 json tag，与前端看到的字段一致
func jsonFieldName(fld reflect.StructField) string {
	name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
	if name == "" || name == "-" {
		return fld.Name
	}
	return name
}

// paramInvalidError 把请求绑定/校验失败标记为参数错误，无法识别具体原因时用 fallback 文案
func paramInvalidError(err error, fallback string) error {
	msg, ok := paramErrorMessage(err)
	if !ok {
		msg = fallback
	}
	return errorx.New(errs.ParamInvalid, "参数错误: "+msg).Wrap(err)
}

// paramErrorMessage 从绑定/校验错误中提取可展示的文案
func paramErrorMessage(err error) (string, bool) {
	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) {
		return formatValidationErrors(verrs), true
	}

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return "请求体不是合法的 JSON", true
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		if typeErr.Field == "" {
			return "请求体字段类型错误", true
		}
		return fmt.Sprintf("%s 类型错误", typeErr.Field), true
	}

	return "", false
}

func formatValidationErrors(verrs validator.ValidationErrors) string {
	msgs := make([]string, 0, len(verrs))
	for i, fe := range verrs {
		if i == maxReportedFields {
			msgs = append(msgs, fmt.Sprintf("共 %d 项", len(verrs)))
			break
		}
		msgs = append(msgs, fmt.Sprintf("%s %s", fieldPath(fe), validationMessage(fe)))
	}
	return strings.Join(msgs, "; ")
}

// fieldPath 用 json 字段名描述出错位置，如 "signer[0].studentId"
func fieldPath(fe validator.FieldError) string {
	ns, structNs := fe.Namespace(), fe.StructNamespace()

	root := structNs
	if i := strings.IndexByte(root, '.'); i >= 0 {
		root = root[:i]
	}
	if root == "" || !strings.HasPrefix(ns, root+".") {
		return ns
	}
	return strings.TrimPrefix(ns, root+".")
}

func validationMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "不能为空"
	case "url":
		return "不是合法的 URL"
	case "oneof":
		return "取值不合法"
	case "len":
		return "长度必须为 " + fe.Param()
	case "max":
		return "长度不能超过 " + fe.Param()
	case "min":
		return "长度不能少于 " + fe.Param()
	case "gte", "lte", "gt", "lt":
		return "超出允许范围"
	case "ltcsfield":
		return "必须早于 " + fe.Param()
	case "gtcsfield":
		return "必须晚于 " + fe.Param()
	case "unique":
		return "存在重复项"
	case "min_signer":
		return "数量不足（至少 " + fe.Param() + " 位）"
	default:
		return "不满足校验规则 " + fe.Tag()
	}
}

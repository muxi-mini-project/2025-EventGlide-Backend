package ginx

import (
	"testing"

	"github.com/raiki02/EG/api/req"
)

func TestValidationMessages(t *testing.T) {
	InitValidation()
	tests := []struct {
		name string
		req  any
		want string
	}{
		{
			name: "required",
			req: struct {
				Title string `json:"title" validate:"required"`
			}{},
			want: "title 不能为空",
		},
		{
			name: "url",
			req: struct {
				AvatarUrl string `json:"avatarUrl" validate:"url"`
			}{AvatarUrl: "not-a-url"},
			want: "avatarUrl 不是合法的 URL",
		},
		{
			name: "oneof",
			req: struct {
				IfRegister string `json:"ifRegister" validate:"oneof=是 否"`
			}{IfRegister: "maybe"},
			want: "ifRegister 取值不合法",
		},
		{
			name: "max",
			req: struct {
				Name string `json:"name" validate:"max=3"`
			}{Name: "abcd"},
			want: "name 长度不能超过 3",
		},
		{
			name: "len",
			req: struct {
				StudentID string `json:"studentId" validate:"len=10"`
			}{StudentID: "1"},
			want: "studentId 长度必须为 10",
		},
		{
			name: "gte",
			req: struct {
				Count int `json:"count" validate:"gte=1"`
			}{},
			want: "count 超出允许范围",
		},
		{
			name: "ltcsfield",
			req: struct {
				Start string `json:"startTime" validate:"ltcsfield=EndTime"`
				End   string `json:"endTime"`
			}{Start: "2026-02-02", End: "2026-01-01"},
			want: "startTime 必须早于 EndTime",
		},
		{
			name: "gtcsfield",
			req: struct {
				Start string `json:"startTime"`
				End   string `json:"endTime" validate:"gtcsfield=StartTime"`
			}{Start: "2026-02-02", End: "2026-01-01"},
			want: "endTime 必须晚于 StartTime",
		},
		{
			name: "unique",
			req: struct {
				Signer []string `json:"signer" validate:"unique"`
			}{Signer: []string{"a", "a"}},
			want: "signer 存在重复项",
		},
		{
			name: "unknown tag",
			req: struct {
				Code string `json:"code" validate:"alphanum"`
			}{Code: "-"},
			want: "code 不满足校验规则 alphanum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, ok := paramErrorMessage(GetValidate().Struct(tt.req))
			if !ok {
				t.Fatalf("paramErrorMessage() ok = false, want true")
			}
			if msg != tt.want {
				t.Fatalf("msg = %q, want %q", msg, tt.want)
			}
		})
	}
}

func TestValidationErrorPaths(t *testing.T) {
	InitValidation()

	type signer struct {
		StudentID string `json:"studentId" validate:"len=10"`
	}
	type diveForm struct {
		Signer []signer `json:"signer" validate:"dive"`
	}
	type nestedForm struct {
		LabelForm struct {
			HolderType string `json:"holderType" validate:"required"`
		} `json:"labelform"`
	}

	msg, ok := paramErrorMessage(GetValidate().Struct(diveForm{
		Signer: []signer{{StudentID: "1"}, {StudentID: "2"}},
	}))
	if !ok {
		t.Fatalf("paramErrorMessage() ok = false, want true")
	}
	want := "signer[0].studentId 长度必须为 10; signer[1].studentId 长度必须为 10"
	if msg != want {
		t.Fatalf("msg = %q, want %q", msg, want)
	}

	msg, ok = paramErrorMessage(GetValidate().Struct(nestedForm{}))
	if !ok {
		t.Fatalf("paramErrorMessage() ok = false, want true")
	}
	want = "labelform.holderType 不能为空"
	if msg != want {
		t.Fatalf("msg = %q, want %q", msg, want)
	}
}

func TestFormatValidationErrorsTruncates(t *testing.T) {
	InitValidation()
	r := struct {
		A string `json:"a" validate:"required"`
		B string `json:"b" validate:"required"`
		C string `json:"c" validate:"required"`
		D string `json:"d" validate:"required"`
		E string `json:"e" validate:"required"`
	}{}

	msg, ok := paramErrorMessage(GetValidate().Struct(r))
	if !ok {
		t.Fatalf("paramErrorMessage() ok = false, want true")
	}
	want := "a 不能为空; b 不能为空; c 不能为空; 共 5 项"
	if msg != want {
		t.Fatalf("msg = %q, want %q", msg, want)
	}
}

// newValidCreateActReq 构造除 signer 外全部合法的创建活动请求
func newValidCreateActReq(holderType string, signers []req.Signer) req.CreateActReq {
	return req.CreateActReq{
		Title:     "title",
		Introduce: "introduce",
		LabelForm: req.CreateActLabel{
			HolderType:    holderType,
			OrganizerUnit: "unit",
			Position:      "position",
			Address:       "address",
			IfRegister:    "是",
			StartTime:     "2026-01-01",
			EndTime:       "2026-01-02",
			ActiveForm:    "form",
			Type:          "type",
			Signer:        signers,
		},
	}
}

func TestMinSignerMessage(t *testing.T) {
	InitValidation()

	tests := []struct {
		name string
		req  req.CreateActReq
		want string
	}{
		{
			name: "personal needs 3 signers",
			req: newValidCreateActReq(HolderTypePersonal, []req.Signer{
				{StudentID: "1234567890"},
				{StudentID: "1234567891"},
			}),
			want: "labelform.signer 数量不足（至少 3 位）",
		},
		{
			name: "organization needs 1 signer",
			req:  newValidCreateActReq("部门", nil),
			want: "labelform.signer 数量不足（至少 1 位）",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, ok := paramErrorMessage(GetValidate().Struct(tt.req))
			if !ok {
				t.Fatalf("paramErrorMessage() ok = false, want true")
			}
			if msg != tt.want {
				t.Fatalf("msg = %q, want %q", msg, tt.want)
			}
		})
	}
}

func TestCreateActReqSignerStudentIDMessage(t *testing.T) {
	InitValidation()
	r := newValidCreateActReq(HolderTypePersonal, []req.Signer{
		{StudentID: "1234567890"},
		{StudentID: "1234567891"},
		{StudentID: "12345"},
	})

	msg, ok := paramErrorMessage(GetValidate().Struct(r))
	if !ok {
		t.Fatalf("paramErrorMessage() ok = false, want true")
	}
	want := "labelform.signer[2].studentId 长度必须为 10"
	if msg != want {
		t.Fatalf("msg = %q, want %q", msg, want)
	}
}

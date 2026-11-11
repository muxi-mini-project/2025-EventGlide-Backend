package req

import (
	"encoding/json"
	"testing"
)

// TestImageUnmarshalJSON 兼容旧版纯 URL 字符串，对象格式与 null 行为保持不变。
func TestImageUnmarshalJSON(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    Image
		wantErr bool
	}{
		{
			name: "对象格式",
			in:   `{"url":"https://img/a.png","width":100,"height":50}`,
			want: Image{Url: "https://img/a.png", Width: 100, Height: 50},
		},
		{
			name: "旧版字符串",
			in:   `"https://img/a.png"`,
			want: Image{Url: "https://img/a.png"},
		},
		{
			name: "null",
			in:   `null`,
			want: Image{},
		},
		{
			name:    "非法类型",
			in:      `123`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got Image
			err := json.Unmarshal([]byte(tc.in), &got)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestCreateActReqAcceptsLegacyShowImg 旧版字符串与新对象混传时都能解码。
func TestCreateActReqAcceptsLegacyShowImg(t *testing.T) {
	var got CreateActReq
	raw := `{"showImg":["https://img/a.png",{"url":"https://img/b.png","width":2,"height":3}]}`
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.ShowImg) != 2 {
		t.Fatalf("showImg length = %d, want 2", len(got.ShowImg))
	}
	if got.ShowImg[0] != (Image{Url: "https://img/a.png"}) {
		t.Fatalf("legacy string decoded to %+v", got.ShowImg[0])
	}
	if got.ShowImg[1] != (Image{Url: "https://img/b.png", Width: 2, Height: 3}) {
		t.Fatalf("object decoded to %+v", got.ShowImg[1])
	}
}

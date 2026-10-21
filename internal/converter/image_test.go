package converter

import (
	"testing"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/internal/model"
)

func TestImageToRespNormalizesFailMark(t *testing.T) {
	imgs := []model.Image{
		{Url: "https://img.example/a.png", Width: -1, Height: -1},
		{Url: "https://img.example/b.png", Width: 800, Height: 600},
	}

	got := ImagesToResp(imgs)
	if got[0].Width != 0 || got[0].Height != 0 {
		t.Errorf("失败标记行 = %+v, want 归一化为 {0 0}", got[0])
	}
	if got[1].Width != 800 || got[1].Height != 600 {
		t.Errorf("正常行 = %+v, want 保持 {800 600}", got[1])
	}
}

// TestImagesFromReqDerivesSizeStatus 落库状态由客户端宽高推导：
// 宽高齐全 → ok 保留原值；任一缺失/非法 → pending 且归零，交由服务端解析。
func TestImagesFromReqDerivesSizeStatus(t *testing.T) {
	imgs := []req.Image{
		{Url: "https://img.example/a.png", Width: 800, Height: 600},
		{Url: "https://img.example/b.png", Width: 100, Height: 0},
		{Url: "https://img.example/c.png", Width: -5, Height: 600},
	}
	got := ImagesFromReq(imgs, 1, "post")
	if got[0].SizeStatus != model.ImageSizeOK || got[0].Width != 800 || got[0].Height != 600 {
		t.Errorf("宽高齐全行 = %+v, want ok 且保留原值", got[0])
	}
	if got[1].SizeStatus != model.ImageSizePending || got[1].Width != 0 || got[1].Height != 0 {
		t.Errorf("部分宽高行 = %+v, want pending 且归零", got[1])
	}
	if got[2].SizeStatus != model.ImageSizePending || got[2].Width != 0 || got[2].Height != 0 {
		t.Errorf("负值行 = %+v, want pending 且归零", got[2])
	}
}

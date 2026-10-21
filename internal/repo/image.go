package repo

import (
	"context"

	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
)

// ImageRepo 图片行仓储，供宽高回填 worker 使用。
type ImageRepo struct {
	d *dao.ImageDao
}

func NewImageRepo(d *dao.ImageDao) *ImageRepo {
	return &ImageRepo{d: d}
}

func (r *ImageRepo) FindZeroSizeImages(ctx context.Context, limit int) ([]model.Image, error) {
	return r.d.FindZeroSizeImages(ctx, limit)
}

func (r *ImageRepo) UpdateImageSize(ctx context.Context, id int64, width, height, status int) error {
	return r.d.UpdateImageSize(ctx, id, width, height, status)
}

func (r *ImageRepo) CopySizeFromDuplicates(ctx context.Context) error {
	return r.d.CopySizeFromDuplicates(ctx)
}

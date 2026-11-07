package dao

import (
	"context"

	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"gorm.io/gorm"
)

// ImageDao 图片行读写，供宽高回填 worker 使用。
type ImageDao struct {
	db *gorm.DB
	l  *logger.LoggerSet
}

func NewImageDao(db *gorm.DB, l *logger.LoggerSet) *ImageDao {
	return &ImageDao{db: db, l: l}
}

// FindZeroSizeImages 查询仍待解析（size_status = 0）的图片行。
// 永久失败行 status = 2 不会被捞起（避免死链每 tick 重试）。
func (d *ImageDao) FindZeroSizeImages(ctx context.Context, limit int) ([]model.Image, error) {
	var imgs []model.Image
	err := d.db.WithContext(ctx).
		Where("size_status = ?", model.ImageSizePending).
		Limit(limit).
		Find(&imgs).Error
	return imgs, err
}

// UpdateImageSize 回写解析结果：成功写宽高 + ok，永久失败写 0/0 + failed。
func (d *ImageDao) UpdateImageSize(ctx context.Context, id int64, width, height, status int) error {
	return d.db.WithContext(ctx).
		Model(&model.Image{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{"width": width, "height": height, "size_status": status}).Error
}

// CopySizeFromDuplicates 同一 URL 已有解析成功（ok）的行时直接复制，避免重复拉图：
// 重复待解析行主要来自不同活动/帖子引用同一张图；草稿保存会先删旧行再插新行。
// 幂等：只动 pending 行；源行要求 status = 1（已解析），避免把错误值复制放大。
func (d *ImageDao) CopySizeFromDuplicates(ctx context.Context) error {
	return d.db.WithContext(ctx).Exec(
		`UPDATE image t JOIN image s
		 ON t.url = s.url AND s.size_status = 1
		 SET t.width = s.width, t.height = s.height, t.size_status = 1
		 WHERE t.size_status = 0`).Error
}

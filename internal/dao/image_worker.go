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

// FindZeroSizeImages 查询宽高仍为 0 的图片行。
// 解析失败的行会被标记为 -1，不会被再次捞起（避免死链每 tick 重试）。
func (d *ImageDao) FindZeroSizeImages(ctx context.Context, limit int) ([]model.Image, error) {
	var imgs []model.Image
	err := d.db.WithContext(ctx).
		// 宽高任一缺失都算待解析；排除已标记 -1 的永久失败行（宽度负值）。
		Where("(width = 0 OR height = 0) AND width >=0 AND height >=0").
		Limit(limit).
		Find(&imgs).Error
	return imgs, err
}

// UpdateImageSize 回写解析出的宽高（失败标记 -1 也走这里）。
func (d *ImageDao) UpdateImageSize(ctx context.Context, id int64, width, height int) error {
	return d.db.WithContext(ctx).
		Model(&model.Image{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{"width": width, "height": height}).Error
}

// CopySizeFromDuplicates 同一 URL 已有其他行解析过宽高时直接复制，避免重复拉图：
// 草稿每次保存都会整批删旧插新，同一 URL 会产生大量重复 0 宽高行。
// 幂等：只动 0 宽高行，源行要求宽高均大于 0。
func (d *ImageDao) CopySizeFromDuplicates(ctx context.Context) error {
	return d.db.WithContext(ctx).Exec(
		`UPDATE image t JOIN image s
		 ON t.url = s.url AND s.width > 0 AND s.height > 0
		 SET t.width = s.width, t.height = s.height
		 WHERE (t.width = 0 OR t.height = 0) AND t.width >=0 AND t.height >= 0`).Error
}

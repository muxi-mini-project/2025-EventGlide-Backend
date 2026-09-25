package dao

import (
	"context"

	"github.com/raiki02/EG/internal/model"
	"gorm.io/gorm"
)

// deleteImagesByOwner 在同一事务内按 (owner_type, owner_id) 删除关联图片。
// image 无外键、无软删字段，删除父行不会级联，须显式清理，否则残留孤儿行并泄漏七牛存储。
func deleteImagesByOwner(c context.Context, tx *gorm.DB, ownerType string, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return tx.WithContext(c).Where("owner_type = ? AND owner_id IN ?", ownerType, ids).Delete(&model.Image{}).Error
}

package service

import (
	"slices"

	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/model"
)

// ActivityVisible 是面向用户的活动可见性唯一实现：详情、活动列表、评论读写与
// 点赞/收藏都走这里（feed 展示判定不在范围内，见 dao.FeedDao.AccessibleActivityBatch）。
// 创建者与签署人不受 effect 限制；其余访客按 effect 分流，未配置按 slow fail-closed。
func ActivityVisible(isChecking, ownerID, viewer string, isSigner bool, effect string) bool {
	if viewer != "" && (ownerID == viewer || isSigner) {
		return true
	}
	if effect == config.EffectFast {
		// 先发后审：与 dao.SetEffect 共用同一个白名单，避免新增审核状态时列表与详情分叉。
		return slices.Contains(model.ActivityCheckingVisibleInFast(), isChecking)
	}
	return isChecking == model.StancePass
}

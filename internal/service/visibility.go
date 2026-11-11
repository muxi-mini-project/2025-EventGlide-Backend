package service

import (
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
		// 先发后审：只挡被驳回的活动。状态白名单而非"非 reject"，
		// 避免历史 NULL/空值被当成可展示状态放给普通访客。
		switch isChecking {
		case model.StancePass, "pending_signers", "pending_auditor":
			return true
		}
		return false
	}
	return isChecking == model.StancePass
}

package dao

import (
	"context"
	"errors"

	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type InteractionDao struct {
	db *gorm.DB
	l  *zap.Logger
}

// ErrInvalidSubject 事件 subject 不在支持范围内，属于永久性错误，消费端可直接丢弃
var ErrInvalidSubject = errors.New("invalid subject")

// ErrSignerDecisionNotAllowed 活动已不在签署人征集阶段，不允许再变更签署意见。
var ErrSignerDecisionNotAllowed = errors.New("signer decision not allowed")

// ErrSignerNotApprover 当前用户不是该活动的签署人。
var ErrSignerNotApprover = errors.New("signer not approver")

func NewInteractionDao(db *gorm.DB, l *logger.LoggerSet) *InteractionDao {
	return &InteractionDao{
		db: db,
		l:  l.Interaction.Named("dao"),
	}
}

func (id *InteractionDao) CommentActivity(c context.Context, studentID string, activityId int64) error {
	return id.db.WithContext(c).Model(&model.Activity{}).Where("id = ?", activityId).
		Update("comment_num", gorm.Expr("comment_num + ?", 1)).Error
}

// DecreaseActivityCommentNum 回减活动评论数；comment_num 为 int unsigned，
// 用 CASE 保证不为负，避免减过头触发 MySQL UNSIGNED 下溢。
func (id *InteractionDao) DecreaseActivityCommentNum(c context.Context, activityId int64, n int64) error {
	return id.db.WithContext(c).Model(&model.Activity{}).Where("id = ?", activityId).
		Update("comment_num", gorm.Expr(
			"CASE WHEN comment_num >= ? THEN comment_num - ? ELSE 0 END", n, n,
		)).Error
}

func (id *InteractionDao) CommentPost(c context.Context, studentID string, postId int64) error {
	return id.db.WithContext(c).Model(&model.Post{}).Where("id = ?", postId).
		Update("comment_num", gorm.Expr("comment_num + ?", 1)).Error
}

// DecreasePostCommentNum 回减帖子评论数；comment_num 为 int unsigned，
// 用 CASE 保证不为负，避免减过头触发 MySQL UNSIGNED 下溢。
func (id *InteractionDao) DecreasePostCommentNum(c context.Context, postId int64, n int64) error {
	return id.db.WithContext(c).Model(&model.Post{}).Where("id = ?", postId).
		Update("comment_num", gorm.Expr(
			"CASE WHEN comment_num >= ? THEN comment_num - ? ELSE 0 END", n, n,
		)).Error
}

func (id *InteractionDao) ApproveActivity(c context.Context, studentID string, activityId int64) error {
	return id.setApprovementStance(c, studentID, activityId, "pass")
}

func (id *InteractionDao) RejectActivity(c context.Context, studentID string, activityId int64) error {
	return id.setApprovementStance(c, studentID, activityId, "reject")
}

// setApprovementStance 在活动仍处于签署人征集中（pending_signers）时更新该签署人的意见。
// 活动一旦进入官方审核或已发布，签署人不再能推翻结论，避免已发布活动被事后改为 reject。
// 同一签署人可在征集阶段内改签（覆盖自己的旧意见）。
// 整个检查-更新放在事务内并对活动行加锁，避免与其它签署人的并发决策交错导致状态错乱。
func (id *InteractionDao) setApprovementStance(c context.Context, studentID string, activityId int64, stance string) error {
	return id.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		var approvement model.Approvement
		if err := tx.Model(&model.Approvement{}).
			Where("student_id = ? AND activity_id = ?", studentID, activityId).
			First(&approvement).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				id.l.Warn("signer not found for activity", zap.String("student_id", studentID), zap.Int64("activity_id", activityId))
				return ErrSignerNotApprover
			}
			id.l.Error("Failed to load approvement", zap.Error(err), zap.String("student_id", studentID), zap.Int64("activity_id", activityId))
			return err
		}

		var act model.Activity
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", activityId).First(&act).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				id.l.Warn("activity not found for signer decision", zap.Int64("activity_id", activityId))
				return ErrSignerDecisionNotAllowed
			}
			id.l.Error("Failed to load activity for signer decision", zap.Error(err), zap.Int64("activity_id", activityId))
			return err
		}
		if act.IsChecking != "pending_signers" {
			id.l.Warn("activity not accepting signer decisions", zap.String("student_id", studentID), zap.Int64("activity_id", activityId))
			return ErrSignerDecisionNotAllowed
		}

		approvement.Stance = stance
		if err := tx.Save(&approvement).Error; err != nil {
			id.l.Error("Failed to update approvement stance", zap.Error(err))
			return err
		}
		return nil
	})
}

// 以下 IsUserLiked/Collected 系列为降级查询：查询失败时返回 false 并记日志，便于发现降级。
func (id *InteractionDao) IsUserLikedActivity(c context.Context, userId, activityId int64) bool {
	var count int64
	if err := id.db.WithContext(c).Model(&model.UserActivityInteraction{}).
		Where("user_id = ? AND activity_id = ? AND type = ?", userId, activityId, "like").Count(&count).Error; err != nil {
		id.l.Error("Failed to check activity like status from db", zap.Error(err), zap.Int64("userId", userId), zap.Int64("activityId", activityId))
	}
	return count > 0
}

func (id *InteractionDao) IsUserCollectedActivity(c context.Context, userId, activityId int64) bool {
	var count int64
	if err := id.db.WithContext(c).Model(&model.UserActivityInteraction{}).
		Where("user_id = ? AND activity_id = ? AND type = ?", userId, activityId, "collect").Count(&count).Error; err != nil {
		id.l.Error("Failed to check activity collect status from db", zap.Error(err), zap.Int64("userId", userId), zap.Int64("activityId", activityId))
	}
	return count > 0
}

func (id *InteractionDao) IsUserLikedPost(c context.Context, userId, postId int64) bool {
	var count int64
	if err := id.db.WithContext(c).Model(&model.UserPostInteraction{}).
		Where("user_id = ? AND post_id = ? AND type = ?", userId, postId, "like").Count(&count).Error; err != nil {
		id.l.Error("Failed to check post like status from db", zap.Error(err), zap.Int64("userId", userId), zap.Int64("postId", postId))
	}
	return count > 0
}

func (id *InteractionDao) IsUserCollectedPost(c context.Context, userId, postId int64) bool {
	var count int64
	if err := id.db.WithContext(c).Model(&model.UserPostInteraction{}).
		Where("user_id = ? AND post_id = ? AND type = ?", userId, postId, "collect").Count(&count).Error; err != nil {
		id.l.Error("Failed to check post collect status from db", zap.Error(err), zap.Int64("userId", userId), zap.Int64("postId", postId))
	}
	return count > 0
}

func (id *InteractionDao) IsUserLikedComment(c context.Context, userId, commentId int64) bool {
	var count int64
	if err := id.db.WithContext(c).Model(&model.UserCommentInteraction{}).
		Where("user_id = ? AND comment_id = ? AND type = ?", userId, commentId, "like").Count(&count).Error; err != nil {
		id.l.Error("Failed to check comment like status from db", zap.Error(err), zap.Int64("userId", userId), zap.Int64("commentId", commentId))
	}
	return count > 0
}

// GetUserLikedCommentIds 批量查询用户点赞的评论 ID
func (id *InteractionDao) GetUserLikedCommentIds(c context.Context, userId int64, commentIds []int64) ([]int64, error) {
	var ids []int64
	err := id.db.WithContext(c).Model(&model.UserCommentInteraction{}).
		Where("user_id = ? AND comment_id IN ? AND type = ?", userId, commentIds, "like").
		Pluck("comment_id", &ids).Error
	return ids, err
}

func (id *InteractionDao) GetUserCollectedActivityIds(c context.Context, userId int64, page, limit int) (*model.PaginatedActivityIds, error) {
	offset := (page - 1) * limit
	var ids []int64
	var total int64

	err := id.db.WithContext(c).Model(&model.UserActivityInteraction{}).
		Where("user_id = ? AND type = ?", userId, "collect").
		Count(&total).Error
	if err != nil {
		return nil, err
	}

	err = id.db.WithContext(c).Model(&model.UserActivityInteraction{}).
		Where("user_id = ? AND type = ?", userId, "collect").
		Order("id DESC").
		Limit(limit).Offset(offset).
		Pluck("activity_id", &ids).Error
	if err != nil {
		return nil, err
	}

	return &model.PaginatedActivityIds{Total: total, Page: page, Limit: limit, Ids: ids}, nil
}

func (id *InteractionDao) GetUserLikedActivityIds(c context.Context, userId int64, page, limit int) (*model.PaginatedActivityIds, error) {
	offset := (page - 1) * limit
	var ids []int64
	var total int64

	err := id.db.WithContext(c).Model(&model.UserActivityInteraction{}).
		Where("user_id = ? AND type = ?", userId, "like").
		Count(&total).Error
	if err != nil {
		return nil, err
	}

	err = id.db.WithContext(c).Model(&model.UserActivityInteraction{}).
		Where("user_id = ? AND type = ?", userId, "like").
		Order("id DESC").
		Limit(limit).Offset(offset).
		Pluck("activity_id", &ids).Error
	if err != nil {
		return nil, err
	}

	return &model.PaginatedActivityIds{Total: total, Page: page, Limit: limit, Ids: ids}, nil
}

func (id *InteractionDao) GetUserCollectedPostIds(c context.Context, userId int64, page, limit int) (*model.PaginatedPostIds, error) {
	offset := (page - 1) * limit
	var ids []int64
	var total int64

	err := id.db.WithContext(c).Model(&model.UserPostInteraction{}).
		Where("user_id = ? AND type = ?", userId, "collect").
		Count(&total).Error
	if err != nil {
		return nil, err
	}

	err = id.db.WithContext(c).Model(&model.UserPostInteraction{}).
		Where("user_id = ? AND type = ?", userId, "collect").
		Order("id DESC").
		Limit(limit).Offset(offset).
		Pluck("post_id", &ids).Error
	if err != nil {
		return nil, err
	}

	return &model.PaginatedPostIds{Total: total, Page: page, Limit: limit, Ids: ids}, nil
}

func (id *InteractionDao) GetUserLikedPostIds(c context.Context, userId int64, page, limit int) (*model.PaginatedPostIds, error) {
	offset := (page - 1) * limit
	var ids []int64
	var total int64

	err := id.db.WithContext(c).Model(&model.UserPostInteraction{}).
		Where("user_id = ? AND type = ?", userId, "like").
		Count(&total).Error
	if err != nil {
		return nil, err
	}

	err = id.db.WithContext(c).Model(&model.UserPostInteraction{}).
		Where("user_id = ? AND type = ?", userId, "like").
		Order("id DESC").
		Limit(limit).Offset(offset).
		Pluck("post_id", &ids).Error
	if err != nil {
		return nil, err
	}

	return &model.PaginatedPostIds{Total: total, Page: page, Limit: limit, Ids: ids}, nil
}

func (id *InteractionDao) GetUserActivityInteractionStatuses(c context.Context, userId int64, activityIds []int64) ([]int64, []int64, error) {
	if len(activityIds) == 0 {
		return []int64{}, []int64{}, nil
	}

	var likedIds []int64
	err := id.db.WithContext(c).Model(&model.UserActivityInteraction{}).
		Where("user_id = ? AND activity_id IN ? AND type = ?", userId, activityIds, "like").
		Pluck("activity_id", &likedIds).Error
	if err != nil {
		return nil, nil, err
	}

	var collectedIds []int64
	err = id.db.WithContext(c).Model(&model.UserActivityInteraction{}).
		Where("user_id = ? AND activity_id IN ? AND type = ?", userId, activityIds, "collect").
		Pluck("activity_id", &collectedIds).Error
	if err != nil {
		return nil, nil, err
	}

	return likedIds, collectedIds, nil
}

func (id *InteractionDao) GetUserPostInteractionStatuses(c context.Context, userId int64, postIds []int64) ([]int64, []int64, error) {
	if len(postIds) == 0 {
		return []int64{}, []int64{}, nil
	}

	var likedIds []int64
	err := id.db.WithContext(c).Model(&model.UserPostInteraction{}).
		Where("user_id = ? AND post_id IN ? AND type = ?", userId, postIds, "like").
		Pluck("post_id", &likedIds).Error
	if err != nil {
		return nil, nil, err
	}

	var collectedIds []int64
	err = id.db.WithContext(c).Model(&model.UserPostInteraction{}).
		Where("user_id = ? AND post_id IN ? AND type = ?", userId, postIds, "collect").
		Pluck("post_id", &collectedIds).Error
	if err != nil {
		return nil, nil, err
	}

	return likedIds, collectedIds, nil
}

// InsertLike 插入点赞记录（Consumer 调用，带事务）
func (id *InteractionDao) InsertLike(ctx context.Context, subject string, subjectID, userID int64) error {
	return id.db.Transaction(func(tx *gorm.DB) error {
		switch subject {
		case "activity":
			var existing model.UserActivityInteraction
			err := tx.WithContext(ctx).Where("user_id = ? AND activity_id = ? AND type = ?", userID, subjectID, "like").First(&existing).Error
			if err == nil {
				return nil // 已存在，幂等返回
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			interaction := &model.UserActivityInteraction{
				Id:         tools.MustGenerateID(),
				UserId:     userID,
				ActivityId: subjectID,
				Type:       "like",
			}
			if err := tx.Create(interaction).Error; err != nil {
				return err
			}
			return tx.Model(&model.Activity{}).Where("id = ?", subjectID).
				Update("like_num", gorm.Expr("like_num + ?", 1)).Error
		case "post":
			var existing model.UserPostInteraction
			err := tx.WithContext(ctx).Where("user_id = ? AND post_id = ? AND type = ?", userID, subjectID, "like").First(&existing).Error
			if err == nil {
				return nil // 已存在，幂等返回
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			interaction := &model.UserPostInteraction{
				Id:     tools.MustGenerateID(),
				UserId: userID,
				PostId: subjectID,
				Type:   "like",
			}
			if err := tx.Create(interaction).Error; err != nil {
				return err
			}
			return tx.Model(&model.Post{}).Where("id = ?", subjectID).
				Update("like_num", gorm.Expr("like_num + ?", 1)).Error
		case "comment":
			var existing model.UserCommentInteraction
			err := tx.WithContext(ctx).Where("user_id = ? AND comment_id = ? AND type = ?", userID, subjectID, "like").First(&existing).Error
			if err == nil {
				return nil // 已存在，幂等返回
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			interaction := &model.UserCommentInteraction{
				Id:        tools.MustGenerateID(),
				UserId:    userID,
				CommentId: subjectID,
				Type:      "like",
			}
			if err := tx.Create(interaction).Error; err != nil {
				return err
			}
			return tx.Model(&model.Comment{}).Where("id = ?", subjectID).
				Update("like_num", gorm.Expr("like_num + ?", 1)).Error
		default:
			return ErrInvalidSubject
		}
	})
}

// DeleteLike 删除点赞记录（Consumer 调用，带事务）
func (id *InteractionDao) DeleteLike(ctx context.Context, subject string, subjectID, userID int64) error {
	return id.db.Transaction(func(tx *gorm.DB) error {
		switch subject {
		case "activity":
			result := tx.WithContext(ctx).Where("user_id = ? AND activity_id = ? AND type = ?", userID, subjectID, "like").
				Delete(&model.UserActivityInteraction{})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return nil // 不存在，幂等返回
			}
			return tx.Model(&model.Activity{}).Where("id = ?", subjectID).
				Update("like_num", gorm.Expr("like_num - ?", 1)).Error
		case "post":
			result := tx.WithContext(ctx).Where("user_id = ? AND post_id = ? AND type = ?", userID, subjectID, "like").
				Delete(&model.UserPostInteraction{})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return nil // 不存在，幂等返回
			}
			return tx.Model(&model.Post{}).Where("id = ?", subjectID).
				Update("like_num", gorm.Expr("like_num - ?", 1)).Error
		case "comment":
			result := tx.WithContext(ctx).Where("user_id = ? AND comment_id = ? AND type = ?", userID, subjectID, "like").
				Delete(&model.UserCommentInteraction{})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return nil // 不存在，幂等返回
			}
			return tx.Model(&model.Comment{}).Where("id = ?", subjectID).
				Update("like_num", gorm.Expr("like_num - ?", 1)).Error
		default:
			return ErrInvalidSubject
		}
	})
}

// InsertCollect 插入收藏记录（Consumer 调用，带事务）
func (id *InteractionDao) InsertCollect(ctx context.Context, subject string, subjectID, userID int64) error {
	return id.db.Transaction(func(tx *gorm.DB) error {
		switch subject {
		case "activity":
			// 先检查是否已存在
			var existing model.UserActivityInteraction
			err := tx.WithContext(ctx).Where("user_id = ? AND activity_id = ? AND type = ?", userID, subjectID, "collect").First(&existing).Error
			if err == nil {
				return nil // 已存在，幂等返回
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			// 不存在则插入
			interaction := &model.UserActivityInteraction{
				Id:         tools.MustGenerateID(),
				UserId:     userID,
				ActivityId: subjectID,
				Type:       "collect",
			}
			if err := tx.Create(interaction).Error; err != nil {
				return err
			}
			return tx.Model(&model.Activity{}).Where("id = ?", subjectID).
				Update("collect_num", gorm.Expr("collect_num + ?", 1)).Error
		case "post":
			// 先检查是否已存在
			var existing model.UserPostInteraction
			err := tx.WithContext(ctx).Where("user_id = ? AND post_id = ? AND type = ?", userID, subjectID, "collect").First(&existing).Error
			if err == nil {
				return nil // 已存在，幂等返回
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			// 不存在则插入
			interaction := &model.UserPostInteraction{
				Id:     tools.MustGenerateID(),
				UserId: userID,
				PostId: subjectID,
				Type:   "collect",
			}
			if err := tx.Create(interaction).Error; err != nil {
				return err
			}
			return tx.Model(&model.Post{}).Where("id = ?", subjectID).
				Update("collect_num", gorm.Expr("collect_num + ?", 1)).Error
		default:
			return ErrInvalidSubject
		}
	})
}

// DeleteCollect 删除收藏记录（Consumer 调用，带事务）
func (id *InteractionDao) DeleteCollect(ctx context.Context, subject string, subjectID, userID int64) error {
	return id.db.Transaction(func(tx *gorm.DB) error {
		switch subject {
		case "activity":
			result := tx.WithContext(ctx).Where("user_id = ? AND activity_id = ? AND type = ?", userID, subjectID, "collect").
				Delete(&model.UserActivityInteraction{})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return nil // 不存在，幂等返回
			}
			return tx.Model(&model.Activity{}).Where("id = ?", subjectID).
				Update("collect_num", gorm.Expr("collect_num - ?", 1)).Error
		case "post":
			result := tx.WithContext(ctx).Where("user_id = ? AND post_id = ? AND type = ?", userID, subjectID, "collect").
				Delete(&model.UserPostInteraction{})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return nil // 不存在，幂等返回
			}
			return tx.Model(&model.Post{}).Where("id = ?", subjectID).
				Update("collect_num", gorm.Expr("collect_num - ?", 1)).Error
		default:
			return ErrInvalidSubject
		}
	})
}

// CountAllLikesFromDB 统计数据库中某目标的所有点赞数（用于对账）
func (id *InteractionDao) CountAllLikesFromDB(ctx context.Context, subject string, subjectID int64) (int64, error) {
	switch subject {
	case "activity":
		var count int64
		err := id.db.WithContext(ctx).Model(&model.UserActivityInteraction{}).
			Where("activity_id = ? AND type = ?", subjectID, "like").Count(&count).Error
		return count, err
	case "post":
		var count int64
		err := id.db.WithContext(ctx).Model(&model.UserPostInteraction{}).
			Where("post_id = ? AND type = ?", subjectID, "like").Count(&count).Error
		return count, err
	case "comment":
		var count int64
		err := id.db.WithContext(ctx).Model(&model.UserCommentInteraction{}).
			Where("comment_id = ? AND type = ?", subjectID, "like").Count(&count).Error
		return count, err
	default:
		return 0, ErrInvalidSubject
	}
}

// CountAllCollectsFromDB 统计数据库中某目标的所有收藏数（用于对账）
func (id *InteractionDao) CountAllCollectsFromDB(ctx context.Context, subject string, subjectID int64) (int64, error) {
	switch subject {
	case "activity":
		var count int64
		err := id.db.WithContext(ctx).Model(&model.UserActivityInteraction{}).
			Where("activity_id = ? AND type = ?", subjectID, "collect").Count(&count).Error
		return count, err
	case "post":
		var count int64
		err := id.db.WithContext(ctx).Model(&model.UserPostInteraction{}).
			Where("post_id = ? AND type = ?", subjectID, "collect").Count(&count).Error
		return count, err
	default:
		return 0, ErrInvalidSubject
	}
}

func (id *InteractionDao) GetRecentActivityIdsWithInteractions(ctx context.Context, limit int, filter func(*model.Activity) bool, out *[]int64) error {
	var activities []model.Activity
	query := id.db.WithContext(ctx).Model(&model.Activity{}).
		Where("like_num > 0 OR collect_num > 0").
		Order("created_at DESC").
		Limit(limit)

	if filter != nil {
		// 先查询所有满足数量条件的活动
		if err := query.Find(&activities).Error; err != nil {
			return err
		}
		// 再过滤
		*out = make([]int64, 0, limit)
		for _, a := range activities {
			if filter(&a) {
				*out = append(*out, a.Id)
			}
		}
	} else {
		if err := query.Pluck("id", out).Error; err != nil {
			return err
		}
	}
	return nil
}

func (id *InteractionDao) GetRecentPostIdsWithInteractions(ctx context.Context, limit int, filter func(*model.Post) bool, out *[]int64) error {
	var posts []model.Post
	query := id.db.WithContext(ctx).Model(&model.Post{}).
		Where("like_num > 0 OR collect_num > 0").
		Order("created_at DESC").
		Limit(limit)

	if filter != nil {
		if err := query.Find(&posts).Error; err != nil {
			return err
		}
		*out = make([]int64, 0, limit)
		for _, p := range posts {
			if filter(&p) {
				*out = append(*out, p.Id)
			}
		}
	} else {
		if err := query.Pluck("id", out).Error; err != nil {
			return err
		}
	}
	return nil
}

func (id *InteractionDao) FixActivityLikeNum(ctx context.Context, activityId int64, likeNum int64) error {
	return id.db.WithContext(ctx).Model(&model.Activity{}).Where("id = ?", activityId).
		Update("like_num", likeNum).Error
}

func (id *InteractionDao) FixActivityCollectNum(ctx context.Context, activityId int64, collectNum int64) error {
	return id.db.WithContext(ctx).Model(&model.Activity{}).Where("id = ?", activityId).
		Update("collect_num", collectNum).Error
}

func (id *InteractionDao) FixPostLikeNum(ctx context.Context, postId int64, likeNum int64) error {
	return id.db.WithContext(ctx).Model(&model.Post{}).Where("id = ?", postId).
		Update("like_num", likeNum).Error
}

func (id *InteractionDao) FixPostCollectNum(ctx context.Context, postId int64, collectNum int64) error {
	return id.db.WithContext(ctx).Model(&model.Post{}).Where("id = ?", postId).
		Update("collect_num", collectNum).Error
}

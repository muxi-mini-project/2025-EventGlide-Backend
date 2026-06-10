package dao

import (
	"context"
	"errors"

	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type InteractionDao struct {
	db *gorm.DB
	l  *zap.Logger
}

func NewInteractionDao(db *gorm.DB, l *logger.LoggerSet) *InteractionDao {
	return &InteractionDao{
		db: db,
		l:  l.Interaction.Named("dao"),
	}
}

func (id *InteractionDao) LikeActivity(c context.Context, studentID string, activityId int64) error {
	var existing model.UserActivityInteraction
	err := id.db.WithContext(c).Where("user_id = (SELECT id FROM user WHERE student_id = ?)", studentID).
		Where("activity_id = ? AND type = ?", activityId, "like").First(&existing).Error
	if err == nil {
		return errors.New("already liked")
	}

	// Create interaction record
	var userId int64
	id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Select("id").Scan(&userId)

	interaction := &model.UserActivityInteraction{
		Id:         tools.MustGenerateID(),
		UserId:     userId,
		ActivityId: activityId,
		Type:       "like",
	}
	if err := id.db.WithContext(c).Create(interaction).Error; err != nil {
		return err
	}

	// Update activity like_num
	return id.db.WithContext(c).Model(&model.Activity{}).Where("id = ?", activityId).
		Update("like_num", gorm.Expr("like_num + ?", 1)).Error
}

func (id *InteractionDao) LikePost(c context.Context, studentID string, postId int64) error {
	var userId int64
	id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Select("id").Scan(&userId)

	interaction := &model.UserPostInteraction{
		Id:     tools.MustGenerateID(),
		UserId: userId,
		PostId: postId,
		Type:   "like",
	}
	if err := id.db.WithContext(c).Create(interaction).Error; err != nil {
		return err
	}

	return id.db.WithContext(c).Model(&model.Post{}).Where("id = ?", postId).
		Update("like_num", gorm.Expr("like_num + ?", 1)).Error
}

func (id *InteractionDao) LikeComment(c context.Context, studentID string, commentId int64) error {
	var userId int64
	id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Select("id").Scan(&userId)

	interaction := &model.UserCommentInteraction{
		Id:        tools.MustGenerateID(),
		UserId:    userId,
		CommentId: commentId,
		Type:      "like",
	}
	if err := id.db.WithContext(c).Create(interaction).Error; err != nil {
		return err
	}

	return id.db.WithContext(c).Model(&model.Comment{}).Where("id = ?", commentId).
		Update("like_num", gorm.Expr("like_num + ?", 1)).Error
}

func (id *InteractionDao) DislikeActivity(c context.Context, studentID string, activityId int64) error {
	var userId int64
	id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Select("id").Scan(&userId)

	if err := id.db.WithContext(c).Where("user_id = ? AND activity_id = ? AND type = ?", userId, activityId, "like").
		Delete(&model.UserActivityInteraction{}).Error; err != nil {
		return err
	}

	return id.db.WithContext(c).Model(&model.Activity{}).Where("id = ?", activityId).
		Update("like_num", gorm.Expr("like_num - ?", 1)).Error
}

func (id *InteractionDao) DislikePost(c context.Context, studentID string, postId int64) error {
	var userId int64
	id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Select("id").Scan(&userId)

	if err := id.db.WithContext(c).Where("user_id = ? AND post_id = ? AND type = ?", userId, postId, "like").
		Delete(&model.UserPostInteraction{}).Error; err != nil {
		return err
	}

	return id.db.WithContext(c).Model(&model.Post{}).Where("id = ?", postId).
		Update("like_num", gorm.Expr("like_num - ?", 1)).Error
}

func (id *InteractionDao) DislikeComment(c context.Context, studentID string, commentId int64) error {
	var userId int64
	id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Select("id").Scan(&userId)

	if err := id.db.WithContext(c).Where("user_id = ? AND comment_id = ? AND type = ?", userId, commentId, "like").
		Delete(&model.UserCommentInteraction{}).Error; err != nil {
		return err
	}

	return id.db.WithContext(c).Model(&model.Comment{}).Where("id = ?", commentId).
		Update("like_num", gorm.Expr("like_num - ?", 1)).Error
}

func (id *InteractionDao) CommentActivity(c context.Context, studentID string, activityId int64) error {
	return id.db.WithContext(c).Model(&model.Activity{}).Where("id = ?", activityId).
		Update("comment_num", gorm.Expr("comment_num + ?", 1)).Error
}

func (id *InteractionDao) CommentPost(c context.Context, studentID string, postId int64) error {
	return id.db.WithContext(c).Model(&model.Post{}).Where("id = ?", postId).
		Update("comment_num", gorm.Expr("comment_num + ?", 1)).Error
}

func (id *InteractionDao) CollectActivity(c context.Context, studentID string, activityId int64) error {
	var userId int64
	id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Select("id").Scan(&userId)

	interaction := &model.UserActivityInteraction{
		Id:         tools.MustGenerateID(),
		UserId:     userId,
		ActivityId: activityId,
		Type:       "collect",
	}
	if err := id.db.WithContext(c).Create(interaction).Error; err != nil {
		return err
	}

	return id.db.WithContext(c).Model(&model.Activity{}).Where("id = ?", activityId).
		Update("collect_num", gorm.Expr("collect_num + ?", 1)).Error
}

func (id *InteractionDao) CollectPost(c context.Context, studentID string, postId int64) error {
	var userId int64
	id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Select("id").Scan(&userId)

	interaction := &model.UserPostInteraction{
		Id:     tools.MustGenerateID(),
		UserId: userId,
		PostId: postId,
		Type:   "collect",
	}
	if err := id.db.WithContext(c).Create(interaction).Error; err != nil {
		return err
	}

	return id.db.WithContext(c).Model(&model.Post{}).Where("id = ?", postId).
		Update("collect_num", gorm.Expr("collect_num + ?", 1)).Error
}

func (id *InteractionDao) DiscollectActivity(c context.Context, studentID string, activityId int64) error {
	var userId int64
	id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Select("id").Scan(&userId)

	if err := id.db.WithContext(c).Where("user_id = ? AND activity_id = ? AND type = ?", userId, activityId, "collect").
		Delete(&model.UserActivityInteraction{}).Error; err != nil {
		return err
	}

	return id.db.WithContext(c).Model(&model.Activity{}).Where("id = ?", activityId).
		Update("collect_num", gorm.Expr("collect_num - ?", 1)).Error
}

func (id *InteractionDao) DiscollectPost(c context.Context, studentID string, postId int64) error {
	var userId int64
	id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Select("id").Scan(&userId)

	if err := id.db.WithContext(c).Where("user_id = ? AND post_id = ? AND type = ?", userId, postId, "collect").
		Delete(&model.UserPostInteraction{}).Error; err != nil {
		return err
	}

	return id.db.WithContext(c).Model(&model.Post{}).Where("id = ?", postId).
		Update("collect_num", gorm.Expr("collect_num - ?", 1)).Error
}

func (id *InteractionDao) ApproveActivity(c context.Context, studentID string, activityId int64) error {
	var approvement model.Approvement
	if err := id.db.WithContext(c).Model(&model.Approvement{}).
		Where("student_id = ? AND activity_id = ?", studentID, activityId).First(&approvement).Error; err != nil {
		id.l.Error("Failed to find approvement", zap.Error(err), zap.String("student_id", studentID), zap.Int64("activity_id", activityId))
		return err
	}
	approvement.Stance = "pass"
	if err := id.db.WithContext(c).Save(&approvement).Error; err != nil {
		id.l.Error("Failed to approve activity", zap.Error(err))
		return err
	}
	return nil
}

func (id *InteractionDao) RejectActivity(c context.Context, studentID string, activityId int64) error {
	var approvement model.Approvement
	if err := id.db.WithContext(c).Model(&model.Approvement{}).
		Where("student_id = ? AND activity_id = ?", studentID, activityId).First(&approvement).Error; err != nil {
		id.l.Error("Failed to find approvement", zap.Error(err))
		return err
	}
	approvement.Stance = "reject"
	if err := id.db.WithContext(c).Save(&approvement).Error; err != nil {
		id.l.Error("Failed to reject activity", zap.Error(err))
		return err
	}
	return nil
}

func (id *InteractionDao) InsertApprovement(c context.Context, studentID, studentName string, activityId int64) error {
	approvement := &model.Approvement{
		Id:          tools.MustGenerateID(),
		StudentId:   studentID,
		StudentName: studentName,
		ActivityId:  activityId,
	}
	if err := id.db.WithContext(c).Create(approvement).Error; err != nil {
		id.l.Error("Failed to insert approvement", zap.Error(err))
		return err
	}
	return nil
}

func (id *InteractionDao) IsUserLikedActivity(c context.Context, userId, activityId int64) bool {
	var count int64
	id.db.WithContext(c).Model(&model.UserActivityInteraction{}).
		Where("user_id = ? AND activity_id = ? AND type = ?", userId, activityId, "like").Count(&count)
	return count > 0
}

func (id *InteractionDao) IsUserCollectedActivity(c context.Context, userId, activityId int64) bool {
	var count int64
	id.db.WithContext(c).Model(&model.UserActivityInteraction{}).
		Where("user_id = ? AND activity_id = ? AND type = ?", userId, activityId, "collect").Count(&count)
	return count > 0
}

func (id *InteractionDao) IsUserLikedPost(c context.Context, userId, postId int64) bool {
	var count int64
	id.db.WithContext(c).Model(&model.UserPostInteraction{}).
		Where("user_id = ? AND post_id = ? AND type = ?", userId, postId, "like").Count(&count)
	return count > 0
}

func (id *InteractionDao) IsUserCollectedPost(c context.Context, userId, postId int64) bool {
	var count int64
	id.db.WithContext(c).Model(&model.UserPostInteraction{}).
		Where("user_id = ? AND post_id = ? AND type = ?", userId, postId, "collect").Count(&count)
	return count > 0
}

func (id *InteractionDao) IsUserLikedComment(c context.Context, userId, commentId int64) bool {
	var count int64
	id.db.WithContext(c).Model(&model.UserCommentInteraction{}).
		Where("user_id = ? AND comment_id = ? AND type = ?", userId, commentId, "like").Count(&count)
	return count > 0
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

package dao

import (
	"context"
	"errors"
	"fmt"

	"github.com/raiki02/EG/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type InteractionDao struct {
	db *gorm.DB
	cd *CommentDao
	ud *UserDao
	ad *ActDao
	pd *PostDao
	l  *zap.Logger
}

func NewInteractionDao(db *gorm.DB, cd *CommentDao, ud *UserDao, ad *ActDao, pd *PostDao, l *zap.Logger) *InteractionDao {
	return &InteractionDao{
		db: db,
		cd: cd,
		ud: ud,
		ad: ad,
		pd: pd,
		l:  l.Named("interaction/dao"),
	}
}

func (id *InteractionDao) LikeActivity(c context.Context, studentID, targetID string) error {
	err1 := id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Update("like_act", gorm.Expr("CONCAT(COALESCE(like_act, ''), ?)", fmt.Sprintf("%s,", targetID)))
	err2 := id.db.Model(&model.Activity{}).Where("bid = ?", targetID).Update("like_num", gorm.Expr("like_num + ?", 1))
	if err1.Error != nil || err2.Error != nil {
		return errors.New("like activity error")
	}
	return nil
}

func (id *InteractionDao) LikePost(c context.Context, studentID, targetID string) error {
	err1 := id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Update("like_post", gorm.Expr("CONCAT(COALESCE(like_post, ''), ?)", fmt.Sprintf("%s,", targetID)))
	err2 := id.db.WithContext(c).Model(&model.Post{}).Where("bid = ?", targetID).Update("like_num", gorm.Expr("like_num + ?", 1))
	if err1.Error != nil || err2.Error != nil {
		return errors.New("like post error")
	}
	return nil
}

func (id *InteractionDao) LikeComment(c context.Context, studentID, targetID string) error {
	err1 := id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Update("like_comment", gorm.Expr("CONCAT(COALESCE(like_comment, ''), ?)", fmt.Sprintf("%s,", targetID)))
	err2 := id.db.WithContext(c).Model(&model.Comment{}).Where("bid = ?", targetID).Update("like_num", gorm.Expr("like_num + ?", 1))
	if err1.Error != nil || err2.Error != nil {
		return errors.New("like comment error")
	}
	return nil
}

func (id *InteractionDao) DislikeActivity(c context.Context, studentID, targetID string) error {
	err1 := id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Update("like_act", gorm.Expr("REPLACE(like_act, ?, '')", fmt.Sprintf("%s,", targetID)))
	err2 := id.db.WithContext(c).Model(&model.Activity{}).Where("bid = ?", targetID).Update("like_num", gorm.Expr("like_num - ?", 1))
	if err1.Error != nil || err2.Error != nil {
		return errors.New("dislike activity error")
	}
	return nil
}

func (id *InteractionDao) DislikePost(c context.Context, studentID, targetID string) error {
	err1 := id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Update("like_post", gorm.Expr("REPLACE(like_post, ?, '')", fmt.Sprintf("%s,", targetID)))
	err2 := id.db.WithContext(c).Model(&model.Post{}).Where("bid = ?", targetID).Update("like_num", gorm.Expr("like_num - ?", 1))
	if err1.Error != nil || err2.Error != nil {
		return errors.New("dislike post error")
	}
	return nil
}

func (id *InteractionDao) DislikeComment(c context.Context, studentID, targetID string) error {
	err1 := id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Update("like_comment", gorm.Expr("REPLACE(like_comment, ?, '')", fmt.Sprintf("%s,", targetID)))
	err2 := id.db.WithContext(c).Model(&model.Comment{}).Where("bid = ?", targetID).Update("like_num", gorm.Expr("like_num - ?", 1)).Error
	if err1.Error != nil || err2 != nil {

		return errors.New("dislike comment error")
	}
	return nil
}

func (id *InteractionDao) CommentActivity(c context.Context, studentID, targetID string) error {
	return id.db.WithContext(c).Model(&model.Activity{}).Where("bid = ?", targetID).Update("comment_num", gorm.Expr("comment_num + ?", 1)).Error
}

func (id *InteractionDao) CommentPost(c context.Context, studentID, targetID string) error {
	return id.db.WithContext(c).Model(&model.Post{}).Where("bid = ?", targetID).Update("comment_num", gorm.Expr("comment_num + ?", 1)).Error
}

func (id *InteractionDao) CommentComment(c context.Context, studentID, targetID string) error {
	return id.db.WithContext(c).Model(&model.Comment{}).Where("bid = ?", targetID).Update("reply_num", gorm.Expr("reply_num + ?", 1)).Error
}

func (id *InteractionDao) CollectActivity(c context.Context, studentID, targetID string) error {
	err1 := id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Update("collect_act", gorm.Expr("CONCAT(COALESCE(collect_act, ''), ?)", fmt.Sprintf("%s,", targetID)))
	err2 := id.db.WithContext(c).Model(&model.Activity{}).Where("bid = ?", targetID).Update("collect_num", gorm.Expr("collect_num + ?", 1))
	if err1.Error != nil || err2.Error != nil {
		return errors.New("collect activity error")
	}
	return nil
}

func (id *InteractionDao) CollectPost(c context.Context, studentID, targetID string) error {
	err1 := id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Update("collect_post", gorm.Expr("CONCAT(COALESCE(collect_post, ''), ?)", fmt.Sprintf("%s,", targetID)))
	err2 := id.db.WithContext(c).Model(&model.Post{}).Where("bid = ?", targetID).Update("collect_num", gorm.Expr("collect_num + ?", 1))
	if err1.Error != nil || err2.Error != nil {
		return errors.New("collect post error")
	}
	return nil
}

func (id *InteractionDao) DiscollectActivity(c context.Context, studentID, targetID string) error {
	err1 := id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Update("collect_act", gorm.Expr("REPLACE(collect_act, ?, '')", fmt.Sprintf("%s,", targetID)))
	err2 := id.db.WithContext(c).Model(&model.Activity{}).Where("bid = ?", targetID).Update("collect_num", gorm.Expr("collect_num - ?", 1))
	if err1.Error != nil || err2.Error != nil {
		return errors.New("discollect activity error")
	}
	return nil

}

func (id *InteractionDao) DiscollectPost(c context.Context, studentID, targetID string) error {
	err1 := id.db.WithContext(c).Model(&model.User{}).Where("student_id = ?", studentID).Update("collect_post", gorm.Expr("REPLACE(collect_post, ?, '')", fmt.Sprintf("%s,", targetID)))
	err2 := id.db.WithContext(c).Model(&model.Post{}).Where("bid = ?", targetID).Update("collect_num", gorm.Expr("collect_num - ?", 1))
	if err1.Error != nil || err2.Error != nil {
		return errors.New("discollect post error")
	}
	return nil
}

func (id *InteractionDao) ApproveActivity(c context.Context, studentID, targetID string) error {
	var approvement model.Approvement
	if err := id.db.WithContext(c).Model(&model.Approvement{}).Where("student_id = ? AND bid = ?", studentID, targetID).First(&approvement).Error; err != nil {
		id.l.Error("Failed to find activity", zap.Error(err), zap.String("student_id", studentID), zap.String("bid", targetID))
		return fmt.Errorf("failed to find activity: %w", err)
	}
	approvement.Stance = "approve"
	if err := id.db.WithContext(c).Save(&approvement).Error; err != nil {
		id.l.Error("Failed to approve activity", zap.Error(err), zap.String("student_id", studentID), zap.String("bid", targetID))
		return fmt.Errorf("failed to approve activity: %w", err)
	}
	return nil
}

func (id *InteractionDao) RejectActivity(c context.Context, studentID, targetID string) error {
	var approvement model.Approvement
	if err := id.db.WithContext(c).Model(&model.Approvement{}).Where("student_id = ? AND bid = ?", studentID, targetID).First(&approvement).Error; err != nil {
		id.l.Error("Failed to find activity", zap.Error(err), zap.String("student_id", studentID), zap.String("bid", targetID))
		return fmt.Errorf("failed to find activity: %w", err)
	}
	approvement.Stance = "reject"
	if err := id.db.WithContext(c).Save(&approvement).Error; err != nil {
		id.l.Error("Failed to reject activity", zap.Error(err), zap.String("student_id", studentID), zap.String("bid", targetID))
		return fmt.Errorf("failed to reject activity: %w", err)
	}
	return nil
}

func (id *InteractionDao) InsertApprovement(c context.Context, studentID, studentName, targetID string) error {
	approvement := &model.Approvement{
		StudentId:   studentID,
		StudentName: studentName,
		Bid:         targetID,
	}
	if err := id.db.WithContext(c).Create(approvement).Error; err != nil {
		id.l.Error("Failed to insert approvement", zap.Error(err), zap.String("student_id", studentID), zap.String("bid", targetID))
		return fmt.Errorf("failed to insert approvement: %w", err)
	}
	return nil
}

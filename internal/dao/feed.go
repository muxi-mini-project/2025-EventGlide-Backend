package dao

import (
	"context"
	"errors"

	"github.com/raiki02/EG/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	TableNameActivity = "activity"
	TableNamePost     = "post"
	TableNameComment  = "comment"
)

type FeedDao struct {
	db *gorm.DB
	l  *zap.Logger
}

func NewFeedDao(db *gorm.DB, l *zap.Logger) *FeedDao {
	return &FeedDao{
		db: db,
		l:  l.Named("feed/dao"),
	}
}

func (fd *FeedDao) CreateFeed(ctx context.Context, feed *model.Feed) error {
	return fd.db.WithContext(ctx).Clauses(clause.OnConflict{
		DoNothing: true,
	}).Create(feed).Error
}

func (fd *FeedDao) GetTotalCnt(ctx context.Context, id string) ([]int, error) {
	var lc, ca int64
	err1 := fd.db.WithContext(ctx).Model(&model.Feed{}).Where("receiver = ? and action in ? and status = ? and student_id != ?", id, []string{"like", "collect"}, "未读", id).Count(&lc).Error
	err2 := fd.db.WithContext(ctx).Model(&model.Feed{}).Where("receiver = ? and action in ? and status = ? and student_id != ?", id, []string{"comment", "at"}, "未读", id).Count(&ca).Error
	if err1 != nil || err2 != nil {
		fd.l.Error("Get Total Cnt Failed", zap.Error(err1), zap.Error(err2))
		return nil, errors.Join(err1, err2)
	}
	return []int{int(lc), int(ca), int(lc + ca)}, nil
}

func (fd *FeedDao) GetLikeFeed(ctx context.Context, id string) ([]*model.Feed, error) {
	var feeds []*model.Feed
	err := fd.db.WithContext(ctx).Where("receiver = ? and action = ? and student_id != ?", id, "like", id).Find(&feeds).Error
	if err != nil {
		fd.l.Error("Get Like Feed Failed", zap.Error(err))
		return nil, err
	}
	return feeds, nil
}

func (fd *FeedDao) GetCollectFeed(ctx context.Context, id string) ([]*model.Feed, error) {
	var feeds []*model.Feed
	err := fd.db.WithContext(ctx).Where("receiver = ? and action = ? and student_id != ?", id, "collect", id).Find(&feeds).Error
	if err != nil {
		fd.l.Error("Get Collect Feed Failed", zap.Error(err))
		return nil, err
	}
	return feeds, nil
}

func (fd *FeedDao) GetCommentFeed(ctx context.Context, id string) ([]*model.Feed, error) {
	var feeds []*model.Feed
	err := fd.db.WithContext(ctx).Where("receiver = ? and action = ? and student_id != ?", id, "comment", id).Find(&feeds).Error
	if err != nil {
		fd.l.Error("Get Comment Feed Failed", zap.Error(err))
		return nil, err
	}
	return feeds, nil
}

func (fd *FeedDao) GetAtFeed(ctx context.Context, id string) ([]*model.Feed, error) {
	var feeds []*model.Feed
	err := fd.db.WithContext(ctx).Where("receiver = ? and action = ? and student_id != ?", id, "at", id).Find(&feeds).Error
	if err != nil {
		fd.l.Error("Get At Feed Failed", zap.Error(err))
		return nil, err
	}
	return feeds, nil
}

func (fd *FeedDao) GetAuditorFeed(ctx context.Context, id string) ([]*model.Approvement, error) {
	var a []*model.Approvement
	if err := fd.db.WithContext(ctx).Where("stance = ? and student_id = ?", "pending", id).Find(&a).Error; err != nil {
		fd.l.Error("Get Auditor Feed Failed", zap.Error(err))
		return nil, err
	}
	return a, nil
}

func (fd *FeedDao) ReadFeedDetail(ctx context.Context, sid, id string) error {
	return fd.db.WithContext(ctx).Model(&model.Feed{}).Where("receiver = ? AND id = ? ", sid, id).Update("status", "已读").Error
}

func (fd *FeedDao) ReadAllFeed(ctx context.Context, sid string) error {
	return fd.db.WithContext(ctx).Model(&model.Feed{}).Where("receiver = ? ", sid).Update("status", "已读").Error
}

func (fd *FeedDao) GetPictureFromObj(ctx context.Context, targetId, object string) (string, error) {
	type Result struct {
		ShowImg string `gorm:"column:show_img"`
	}
	var tableName string
	switch object {
	case TableNameActivity:
		tableName = TableNameActivity
	case TableNamePost:
		tableName = TableNamePost
	default:
		return "", errors.New("invalid object type")
	}
	var res Result
	err := fd.db.WithContext(ctx).Table(tableName).Where("bid = ?", targetId).Select("show_img").Find(&res).Error
	if err != nil {
		fd.l.Error("Get First Pic Failed", zap.Error(err))
		return "", err
	}

	return res.ShowImg, nil
}

func (fd *FeedDao) ResolveRootIDByCommentID(ctx context.Context, commentID string) (string, error) {
	curID := commentID
	for i := 0; i < 20; i++ {
		var cmt model.Comment
		if err := fd.db.WithContext(ctx).Where("bid = ?", curID).First(&cmt).Error; err != nil {
			return "", err
		}

		switch cmt.Subject {
		case TableNamePost, TableNameActivity:
			return cmt.ParentID, nil
		case TableNameComment:
			if cmt.ParentID == "" {
				return "", errors.New("comment parent id is empty")
			}
			curID = cmt.ParentID
		default:
			return "", errors.New("unknown comment subject")
		}
	}

	return "", errors.New("comment chain too deep")
}

func (fd *FeedDao) GetPictureFromRootID(ctx context.Context, rootID string) (string, error) {
	if pic, ok, err := fd.findShowImgByTable(ctx, TableNamePost, rootID); err != nil {
		return "", err
	} else if ok {
		return pic, nil
	}

	if pic, ok, err := fd.findShowImgByTable(ctx, TableNameActivity, rootID); err != nil {
		return "", err
	} else if ok {
		return pic, nil
	}

	return "", gorm.ErrRecordNotFound
}

func (fd *FeedDao) ResolveRootSubjectByID(ctx context.Context, rootID string) (string, error) {
	if ok, err := fd.existsByTableAndBid(ctx, TableNamePost, rootID); err != nil {
		return "", err
	} else if ok {
		return TableNamePost, nil
	}

	if ok, err := fd.existsByTableAndBid(ctx, TableNameActivity, rootID); err != nil {
		return "", err
	} else if ok {
		return TableNameActivity, nil
	}

	return "", gorm.ErrRecordNotFound
}

func (fd *FeedDao) findShowImgByTable(ctx context.Context, tableName, bid string) (string, bool, error) {
	type Result struct {
		ShowImg string `gorm:"column:show_img"`
	}
	var res Result
	err := fd.db.WithContext(ctx).Table(tableName).Where("bid = ?", bid).Select("show_img").Take(&res).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		fd.l.Error("Get First Pic Failed", zap.Error(err), zap.String("table", tableName), zap.String("bid", bid))
		return "", false, err
	}
	return res.ShowImg, true, nil
}

func (fd *FeedDao) existsByTableAndBid(ctx context.Context, tableName, bid string) (bool, error) {
	var cnt int64
	err := fd.db.WithContext(ctx).Table(tableName).Where("bid = ?", bid).Count(&cnt).Error
	if err != nil {
		fd.l.Error("Check Record Exists Failed", zap.Error(err), zap.String("table", tableName), zap.String("bid", bid))
		return false, err
	}
	return cnt > 0, nil
}

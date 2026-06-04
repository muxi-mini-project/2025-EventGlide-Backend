package service

import (
	"context"
	"errors"

	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/mq"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/pkg/logger"
	"github.com/raiki02/EG/pkg/utils"
	"go.uber.org/zap"
)

var _ CommentServiceHdl = &CommentService{}

type CommentServiceHdl interface {
	CreateComment(c context.Context, cmt *model.Comment, studentID string) (*model.Comment, error)
	DeleteComment(c context.Context, targetID, studentID string) error
	AnswerComment(c context.Context, cmt *model.Comment, studentID string) (*model.Comment, error)
	LoadComments(c context.Context, parentID string) ([]model.Comment, error)
	EnrichComments(c context.Context, cmts []model.Comment, viewerID string) []model.CommentDetail
	EnrichComment(c context.Context, cmt *model.Comment, viewerID string) model.CommentDetail
	EnrichReply(c context.Context, cmt *model.Comment, viewerID string) model.ReplyDetail
}

type CommentService struct {
	cd *dao.CommentDao
	ud *repo.UserRepo
	id *repo.InteractionRepo
	mq mq.MQHdl
	sg SubjectGetter
	l  *zap.Logger
}

func NewCommentService(cd *dao.CommentDao, ud *repo.UserRepo, id *repo.InteractionRepo, mq mq.MQHdl, sg SubjectGetter, l *logger.LoggerSet) *CommentService {
	return &CommentService{
		cd: cd,
		ud: ud,
		id: id,
		mq: mq,
		sg: sg,
		l:  l.Comment.Named("service"),
	}
}

func (cs *CommentService) CreateComment(c context.Context, cmt *model.Comment, studentID string) (*model.Comment, error) {
	creator, err := cs.ud.GetUserInfo(c, studentID)
	if err != nil {
		cs.l.Error("Error get user info failed", zap.Error(err), zap.String("studentID", studentID))
		return nil, err
	}
	cmt.CreatorName = creator.Name
	cmt.CreatorAvatar = creator.Avatar

	if cmt.Subject == SubjectComment {
		parent := cs.cd.FindCmtByID(c, cmt.ParentID)
		if parent == nil {
			return nil, errors.New("comment parent not found")
		}
		cmt.RootID = parent.Bid
		cmt.RootObjectID = parent.RootObjectID
		cmt.RootObjectType = parent.RootObjectType
	} else {
		cmt.RootObjectID = cmt.ParentID
		cmt.RootObjectType = cmt.Subject
	}

	rootID := cmt.RootObjectID
	rootType := cmt.RootObjectType

	err = cs.cd.CreateComment(c, cmt)
	cs.l.Info("CreateComment",
		zap.String("bid", cmt.Bid),
		zap.String("studentid", cmt.StudentID),
		zap.String("parentid", cmt.ParentID),
	)
	if err != nil {
		cs.l.Error("Error comment create failed", zap.Error(err))
		return nil, err
	}

	subject, err := cs.sg.GetSubjectInfo(c, cmt.ParentID, cmt.Subject)
	if err != nil {
		cs.l.Error("Error get activity or post or comment failed", zap.Error(err))
		return nil, err
	}

	// TODO 优雅实现
	if studentID == subject.StudentID {
		return cmt, nil
	}

	f := model.Feed{
		StudentID: studentID,
		TargetBid: cmt.ParentID,
		Object:    cmt.Subject,
		Action:    "comment",
		Receiver:  subject.StudentID,
		RootID:    rootID,
		RootType:  rootType,
	}

	err = cs.mq.Publish(c, "feed_stream", f)
	if err != nil {
		cs.l.Error("Publish Comment Feed Failed", zap.Error(err), zap.Any("feed", f))
	} else {
		cs.l.Info("Publish Comment Feed Success", zap.Any("feed", f))
	}

	switch cmt.Subject {
	case SubjectActivity:
		err = cs.id.CommentActivity(c, studentID, cmt.ParentID)
	case SubjectPost:
		err = cs.id.CommentPost(c, studentID, cmt.ParentID)
	case SubjectComment:
		err = cs.id.CommentComment(c, studentID, cmt.ParentID)
	}
	if err != nil {
		cs.l.Error("Error comment create failed", zap.Error(err))
		return nil, err
	}

	return cmt, nil
}

func (cs *CommentService) DeleteComment(c context.Context, targetID, studentID string) error {
	cmt := cs.cd.FindCmtByID(c, targetID)
	if cmt != nil && cmt.Subject == SubjectComment && cmt.RootID != "" {
		if err := cs.cd.DecrementReplyNum(c, cmt.RootID); err != nil {
			cs.l.Error("Error decrement reply num", zap.Error(err))
		}
	}
	return cs.cd.DeleteComment(c, studentID, targetID)
}

func (cs *CommentService) AnswerComment(c context.Context, cmt *model.Comment, studentID string) (*model.Comment, error) {
	creator, err := cs.ud.GetUserInfo(c, studentID)
	if err != nil {
		cs.l.Error("Error get user info failed", zap.Error(err), zap.String("studentID", studentID))
		return nil, err
	}
	cmt.CreatorName = creator.Name
	cmt.CreatorAvatar = creator.Avatar

	parentCmt := cs.cd.FindCmtByID(c, cmt.ParentID)
	if parentCmt == nil {
		return nil, errors.New("comment parent not found")
	}
	cmt.RootID = parentCmt.Bid
	cmt.RootObjectID = parentCmt.RootObjectID
	cmt.RootObjectType = parentCmt.RootObjectType
	cmt.ReplyToUserID = parentCmt.StudentID
	cmt.ReplyToUserName = parentCmt.CreatorName

	rootID := parentCmt.RootObjectID
	rootType := parentCmt.RootObjectType

	err = cs.cd.AnswerComment(c, cmt)
	if err != nil {
		cs.l.Error("Error comment answer failed", zap.Error(err))
		return nil, err
	}
	cs.l.Info("AnswerComment",
		zap.String("bid", cmt.Bid),
		zap.String("studentid", cmt.StudentID),
	)

	parent, err := cs.sg.GetSubjectInfo(c, cmt.ParentID, cmt.Subject)
	if err != nil {
		cs.l.Error("Error get activity or post or comment failed", zap.Error(err))
		return nil, err
	}

	root, err := cs.sg.GetSubjectInfo(c, rootID, rootType)
	if err != nil {
		cs.l.Error("Error get activity or post or comment failed", zap.Error(err))
		return nil, err
	}

	if err = cs.IncreaseCommentNum(c, root, studentID); err != nil {
		cs.l.Error("Error increase comment num failed", zap.Error(err))
		return nil, err
	}

	if studentID == parent.StudentID {
		return cmt, nil
	}

	f := model.Feed{
		StudentID: studentID,
		TargetBid: cmt.ParentID,
		Object:    "comment",
		Action:    "at",
		Receiver:  parent.StudentID,
		RootID:    rootID,
		RootType:  rootType,
	}

	err = cs.mq.Publish(c, "feed_stream", f)
	if err != nil {
		cs.l.Error("Publish Comment Feed Failed", zap.Error(err), zap.Any("feed", f))
	} else {
		cs.l.Info("Publish Comment Feed Success", zap.Any("feed", f))
	}

	return cmt, nil
}

func (cs *CommentService) resolveCommentRootMeta(c context.Context, commentID string) (string, string, string, error) {
	cur := cs.cd.FindCmtByID(c, commentID)
	if cur == nil {
		return "", "", "", errors.New("comment not found")
	}

	rootCommentID := cur.RootID
	if rootCommentID == "" {
		rootCommentID = cur.Bid
	}

	for i := 0; i < 20; i++ {
		switch cur.Subject {
		case SubjectActivity, SubjectPost:
			return rootCommentID, cur.ParentID, cur.Subject, nil
		case SubjectComment:
			if cur.ParentID == "" {
				return "", "", "", errors.New("comment parent id is empty")
			}
			cur = cs.cd.FindCmtByID(c, cur.ParentID)
			if cur == nil {
				return "", "", "", errors.New("comment parent not found")
			}
		default:
			return "", "", "", errors.New("unknown comment subject")
		}
	}

	return "", "", "", errors.New("comment chain too deep")
}

func (cs *CommentService) LoadComments(c context.Context, parentID string) ([]model.Comment, error) {
	cmts, err := cs.cd.LoadComments(c, parentID)
	if err != nil {
		cs.l.Error("Error load comments failed", zap.Error(err))
		return nil, err
	}
	return cmts, nil
}

func (cs *CommentService) EnrichComments(c context.Context, cmts []model.Comment, viewerID string) []model.CommentDetail {
	if len(cmts) == 0 {
		return nil
	}

	idSet := make(map[string]struct{})
	idSet[viewerID] = struct{}{}
	for _, cmt := range cmts {
		idSet[cmt.StudentID] = struct{}{}
	}

	for _, cmt := range cmts {
		replies, _ := cs.cd.LoadAnswers(c, cmt.Bid)
		for _, reply := range replies {
			idSet[reply.StudentID] = struct{}{}
		}
	}

	idList := make([]string, 0, len(idSet))
	for id := range idSet {
		idList = append(idList, id)
	}
	userMap, err := cs.ud.GetUsersByIDs(c, idList)
	if err != nil {
		cs.l.Error("Error batch get users", zap.Error(err))
	}

	details := make([]model.CommentDetail, 0, len(cmts))
	for i := range cmts {
		details = append(details, cs.enrichCommentWithCache(c, &cmts[i], viewerID, userMap))
	}
	return details
}

func (cs *CommentService) EnrichComment(c context.Context, cmt *model.Comment, viewerID string) model.CommentDetail {
	idList := []string{viewerID, cmt.StudentID}
	userMap, _ := cs.ud.GetUsersByIDs(c, idList)
	return cs.enrichCommentWithCache(c, cmt, viewerID, userMap)
}

func (cs *CommentService) EnrichReply(c context.Context, cmt *model.Comment, viewerID string) model.ReplyDetail {
	idList := []string{viewerID, cmt.StudentID}
	userMap, _ := cs.ud.GetUsersByIDs(c, idList)
	return cs.enrichReplyWithCache(c, cmt, viewerID, userMap)
}

func (cs *CommentService) enrichCommentWithCache(c context.Context, cmt *model.Comment, viewerID string, userMap map[string]*model.User) model.CommentDetail {
	creator := userMap[cmt.StudentID]
	viewer := userMap[viewerID]

	replies, err := cs.cd.LoadAnswers(c, cmt.Bid)
	if err != nil {
		cs.l.Error("Error load answers when enriching comment", zap.Error(err))
		return model.CommentDetail{}
	}

	detail := model.CommentDetail{
		Comment: *cmt,
	}
	if creator != nil {
		detail.Creator = model.UserBrief{
			StudentID: creator.StudentID,
			Name:      creator.Name,
			Avatar:    creator.Avatar,
		}
	}
	if viewer != nil {
		detail.IsLike = utils.HasLike(viewer.LikeComment, cmt.Bid)
	}
	for _, reply := range replies {
		detail.Replies = append(detail.Replies, cs.enrichReplyWithCache(c, &reply, viewerID, userMap))
	}
	return detail
}

func (cs *CommentService) enrichReplyWithCache(c context.Context, cmt *model.Comment, viewerID string, userMap map[string]*model.User) model.ReplyDetail {
	viewer := userMap[viewerID]

	isLike := false
	if viewer != nil {
		isLike = utils.HasLike(viewer.LikeComment, cmt.Bid)
	}

	return model.ReplyDetail{
		Comment:        *cmt,
		ParentUserName: cmt.ReplyToUserName,
		IsLike:         isLike,
	}
}

func (cs *CommentService) IncreaseCommentNum(ctx context.Context, subject SubjectInfo, commenterID string) error {
	switch subject.Subject {
	case SubjectActivity:
		return cs.id.CommentActivity(ctx, commenterID, subject.Bid)
	case SubjectPost:
		return cs.id.CommentPost(ctx, commenterID, subject.Bid)
	case SubjectComment:
		return cs.id.CommentComment(ctx, commenterID, subject.Bid)
	}

	return errors.New("invalid subject")
}

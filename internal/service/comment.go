package service

import (
	"context"
	"errors"
	"strings"

	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/mq"
	"github.com/raiki02/EG/internal/repo"
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
	cd  *dao.CommentDao
	ud  *repo.UserRepo
	id  *repo.InteractionRepo
	mq  mq.MQHdl
	apg ActPostCommentGetter
	l   *zap.Logger
}

func NewCommentService(cd *dao.CommentDao, ud *repo.UserRepo, id *repo.InteractionRepo, l *zap.Logger, mq mq.MQHdl,
	apg ActPostCommentGetter) *CommentService {
	return &CommentService{
		cd:  cd,
		ud:  ud,
		id:  id,
		mq:  mq,
		apg: apg,
		l:   l.Named("comment/service"),
	}
}

func (cs *CommentService) CreateComment(c context.Context, cmt *model.Comment, studentID string) (*model.Comment, error) {
	rootID := ""
	rootType := ""
	if cmt.Subject == SubjectComment {
		rootCommentID, resolvedRootID, resolvedRootType, resolveErr := cs.resolveCommentRootMeta(c, cmt.ParentID)
		if resolveErr != nil {
			cs.l.Error("Error resolve comment root meta failed", zap.Error(resolveErr), zap.String("parentID", cmt.ParentID))
			return nil, resolveErr
		}
		cmt.RootID = rootCommentID
		rootID = resolvedRootID
		rootType = resolvedRootType
	}

	err := cs.cd.CreateComment(c, cmt)
	cs.l.Info("CreateComment",
		zap.String("bid", cmt.Bid),
		zap.String("studentid", cmt.StudentID),
		zap.String("parentid", cmt.ParentID),
	)
	if err != nil {
		cs.l.Error("Error comment create failed", zap.Error(err))
		return nil, err
	}

	ap, err := cs.apg.GetActivityOrPostOrComment(c, cmt.ParentID, cmt.Subject)
	if err != nil {
		cs.l.Error("Error get activity or post or comment failed", zap.Error(err))
		return nil, err
	}

	// TODO 优雅实现
	if studentID == ap.GetStudentID() {
		return cmt, nil
	}

	f := model.Feed{
		StudentID: studentID,
		TargetBid: cmt.ParentID,
		Object:    cmt.Subject,
		Action:    "comment",
		Receiver:  ap.GetStudentID(),
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
	case "activity":
		err = cs.id.CommentActivity(c, studentID, cmt.ParentID)
	case "post":
		err = cs.id.CommentPost(c, studentID, cmt.ParentID)
	case "comment":
		err = cs.id.CommentComment(c, studentID, cmt.ParentID)
	}
	if err != nil {
		cs.l.Error("Error comment create failed", zap.Error(err))
		return nil, err
	}

	return cmt, nil
}

func (cs *CommentService) DeleteComment(c context.Context, targetID, studentID string) error {
	err := cs.cd.DeleteComment(c, studentID, targetID)
	if err != nil {
		cs.l.Error("Error comment delete failed", zap.Error(err))
		return err
	}
	return nil
}

func (cs *CommentService) AnswerComment(c context.Context, cmt *model.Comment, studentID string) (*model.Comment, error) {
	rootCommentID, rootID, rootType, err := cs.resolveCommentRootMeta(c, cmt.ParentID)
	if err != nil {
		cs.l.Error("Error resolve comment root meta failed", zap.Error(err), zap.String("parentID", cmt.ParentID))
		return nil, err
	}
	cmt.RootID = rootCommentID

	err = cs.cd.AnswerComment(c, cmt)
	if err != nil {
		cs.l.Error("Error comment answer failed", zap.Error(err))
		return nil, err
	}
	cs.l.Info("AnswerComment",
		zap.String("bid", cmt.Bid),
		zap.String("studentid", cmt.StudentID),
	)

	ap, err := cs.apg.GetActivityOrPostOrComment(c, cmt.ParentID, cmt.Subject)
	if err != nil {
		cs.l.Error("Error get activity or post or comment failed", zap.Error(err))
		return nil, err
	}

	ap2, err := cs.apg.GetActivityOrPostOrComment(c, rootID, rootType)
	if err != nil {
		cs.l.Error("Error get activity or post or comment failed", zap.Error(err))
		return nil, err
	}

	if err = cs.IncreaseCommentNum(c, &ap2); err != nil {
		cs.l.Error("Error increase comment num failed", zap.Error(err))
		return nil, err
	}

	if studentID == ap.GetStudentID() {
		return cmt, nil
	}

	f := model.Feed{
		StudentID: studentID,
		TargetBid: cmt.ParentID,
		Object:    "comment",
		Action:    "at",
		Receiver:  ap.GetStudentID(),
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
	details := make([]model.CommentDetail, 0, len(cmts))
	for i := range cmts {
		details = append(details, cs.enrichComment(c, &cmts[i], viewerID))
	}
	return details
}

func (cs *CommentService) EnrichComment(c context.Context, cmt *model.Comment, viewerID string) model.CommentDetail {
	return cs.enrichComment(c, cmt, viewerID)
}

func (cs *CommentService) EnrichReply(c context.Context, cmt *model.Comment, viewerID string) model.ReplyDetail {
	return cs.enrichReply(c, cmt, viewerID)
}

func (cs *CommentService) enrichComment(c context.Context, cmt *model.Comment, viewerID string) model.CommentDetail {
	user, err := cs.ud.GetUserInfo(c, cmt.StudentID)
	if err != nil {
		cs.l.Error("Error get user info when enriching comment", zap.Error(err))
		return model.CommentDetail{}
	}
	searcher, err := cs.ud.GetUserInfo(c, viewerID)
	if err != nil {
		cs.l.Error("Error get user info when enriching comment", zap.Error(err))
		return model.CommentDetail{}
	}

	replies, err := cs.cd.LoadAnswers(c, cmt.Bid)
	if err != nil {
		cs.l.Error("Error load answers when enriching comment", zap.Error(err))
		return model.CommentDetail{}
	}

	detail := model.CommentDetail{
		Comment: *cmt,
		Creator: model.UserBrief{
			StudentID: user.StudentID,
			Name:      user.Name,
			Avatar:    user.Avatar,
		},
		IsLike: strings.Contains(searcher.LikeComment, cmt.Bid),
	}
	for _, reply := range replies {
		detail.Replies = append(detail.Replies, cs.enrichReply(c, &reply, viewerID))
	}
	return detail
}

func (cs *CommentService) enrichReply(c context.Context, cmt *model.Comment, viewerID string) model.ReplyDetail {
	user, err := cs.ud.GetUserInfo(c, cmt.StudentID)
	if err != nil {
		cs.l.Error("Error get user info when enriching reply", zap.Error(err))
		return model.ReplyDetail{}
	}
	searcher, err := cs.ud.GetUserInfo(c, viewerID)
	if err != nil {
		cs.l.Error("Error get user info when enriching reply", zap.Error(err))
		return model.ReplyDetail{}
	}

	pc := cs.cd.FindCmtByID(c, cmt.ParentID)
	if pc == nil {
		cs.l.Error("Error find comment by id", zap.String("pid", cmt.ParentID))
		return model.ReplyDetail{}
	}
	pu, err := cs.ud.GetUserInfo(c, pc.StudentID)
	if err != nil {
		cs.l.Error("Error get user info when enriching reply", zap.Error(err))
		return model.ReplyDetail{}
	}

	return model.ReplyDetail{
		Comment: *cmt,
		Creator: model.UserBrief{
			StudentID: user.StudentID,
			Name:      user.Name,
			Avatar:    user.Avatar,
		},
		ParentUserName: pu.Name,
		IsLike:         strings.Contains(searcher.LikeComment, cmt.Bid),
	}
}

func (cs *CommentService) IncreaseCommentNum(c context.Context, parent *ActPostCommentWrapper) error {
	studentID := parent.GetStudentID()
	bid := parent.GetBid()
	switch {
	case parent.Activity != nil:
		return cs.id.CommentActivity(c, studentID, bid)
	case parent.Post != nil:
		return cs.id.CommentPost(c, studentID, bid)
	case parent.Comment != nil:
		return cs.id.CommentComment(c, studentID, bid)
	default:
		return nil
	}
}

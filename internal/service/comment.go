package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/mq"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
)

type CommentServiceHdl interface {
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

func (cs *CommentService) toComment(r req.CreateCommentReq, studentId string) *model.Comment {
	return &model.Comment{
		StudentID: studentId,
		Content:   r.Content,
		ParentID:  r.ParentID,
		CreatedAt: time.Now(),
		Bid:       tools.GenUUID(),
		Position:  "华中师范大学",
		Subject:   r.Subject,
	}
}

func (cs *CommentService) CreateComment(c context.Context, r req.CreateCommentReq, studentId string) (resp.CommentResp, error) {
	cmt := cs.toComment(r, studentId)
	rootID := ""
	rootType := ""
	if r.Subject == SubjectComment {
		rootCommentID, resolvedRootID, resolvedRootType, resolveErr := cs.resolveCommentRootMeta(c, r.ParentID)
		if resolveErr != nil {
			cs.l.Error("Error resolve comment root meta failed", zap.Error(resolveErr), zap.String("parentID", r.ParentID))
			return resp.CommentResp{}, resolveErr
		}
		cmt.RootId = rootCommentID
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
		return resp.CommentResp{}, err
	}

	ap, err := cs.apg.GetActivityOrPostOrComment(c, r.ParentID, r.Subject)
	if err != nil {
		cs.l.Error("Error get activity or post or comment failed", zap.Error(err))
		return resp.CommentResp{}, err
	}

	f := model.Feed{
		StudentId: studentId,
		TargetBid: r.ParentID,
		Object:    r.Subject,
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

	// 评论数+1
	switch r.Subject {
	case "activity":
		err = cs.id.CommentActivity(c, studentId, r.ParentID)
	case "post":
		err = cs.id.CommentPost(c, studentId, r.ParentID)
	case "comment":
		err = cs.id.CommentComment(c, studentId, r.ParentID)
	}
	if err != nil {
		cs.l.Error("Error comment create failed", zap.Error(err))
		return resp.CommentResp{}, err
	}

	return cs.toResp(c, cmt, studentId), nil
}

func (cs *CommentService) DeleteComment(c context.Context, targetId, studentId string) error {
	err := cs.cd.DeleteComment(c, studentId, targetId)
	if err != nil {
		cs.l.Error("Error comment delete failed", zap.Error(err))
		return err
	}
	return nil
}

// 二级评论
func (cs *CommentService) AnswerComment(c context.Context, r req.CreateCommentReq, studentId string) (resp.ReplyResp, error) {
	cmt := cs.toComment(r, studentId)
	rootCommentID, rootID, rootType, err := cs.resolveCommentRootMeta(c, r.ParentID)
	if err != nil {
		cs.l.Error("Error resolve comment root meta failed", zap.Error(err), zap.String("parentID", r.ParentID))
		return resp.ReplyResp{}, err
	}
	cmt.RootId = rootCommentID

	err = cs.cd.AnswerComment(c, cmt)
	if err != nil {
		cs.l.Error("Error comment answer failed", zap.Error(err))
		return resp.ReplyResp{}, err
	}
	cs.l.Info("AnswerComment",
		zap.String("bid", cmt.Bid),
		zap.String("studentid", cmt.StudentID),
	)

	ap, err := cs.apg.GetActivityOrPostOrComment(c, r.ParentID, r.Subject)
	if err != nil {
		cs.l.Error("Error get activity or post or comment failed", zap.Error(err))
		return resp.ReplyResp{}, err
	}

	ap2, err := cs.apg.GetActivityOrPostOrComment(c, rootID, rootType)
	if err != nil {
		cs.l.Error("Error get activity or post or comment failed", zap.Error(err))
		return resp.ReplyResp{}, err
	}

	if err = cs.IncreaseCommentNum(c, &ap2); err != nil {
		cs.l.Error("Error increase comment num failed", zap.Error(err))
		return resp.ReplyResp{}, err
	}

	f := model.Feed{
		StudentId: studentId,
		TargetBid: r.ParentID,
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

	return cs.toReply(c, cmt, studentId), nil
}

func (cs *CommentService) resolveCommentRootMeta(c context.Context, commentID string) (string, string, string, error) {
	cur := cs.cd.FindCmtByID(c, commentID)
	if cur == nil {
		return "", "", "", errors.New("comment not found")
	}

	rootCommentID := cur.RootId
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

func (cs *CommentService) LoadComments(c context.Context, parentid string, studentId string) ([]resp.CommentResp, error) {
	// 加载一级评论
	cmts, err := cs.cd.LoadComments(c, parentid)
	if err != nil {
		cs.l.Error("Error load comments failed", zap.Error(err))
		return nil, err
	}
	res := cs.toResps(c, cmts, studentId)
	return res, nil
}

func (cs *CommentService) toResp(c context.Context, cmt *model.Comment, studentId string) resp.CommentResp {
	var res resp.CommentResp                         //返回值
	user, err := cs.ud.GetUserInfo(c, cmt.StudentID) //该条评论用户信息
	if err != nil {
		cs.l.Error("Error get user info when comment to resp", zap.Error(err))
		return resp.CommentResp{}
	}
	searcher, err := cs.ud.GetUserInfo(c, studentId) //当前用户信息
	if err != nil {
		cs.l.Error("Error get user info when comment to resp", zap.Error(err))
		return resp.CommentResp{}
	}
	// 该条评论下的所有评论, 不分级
	replys, err := cs.cd.LoadAnswers(c, cmt.Bid) //该条评论的回复（存储模型）
	if err != nil {
		cs.l.Error("Error load answers when loading replies", zap.Error(err))
		return resp.CommentResp{}
	}
	if strings.Contains(searcher.LikeComment, cmt.Bid) {
		res.IsLike = "true"
	} else {
		res.IsLike = "false"
	}
	res.Content = cmt.Content
	res.CommentedTime = tools.ParseTime(cmt.CreatedAt)
	res.Bid = cmt.Bid
	res.ParentID = cmt.ParentID
	res.RootID = cmt.RootId
	res.CommentedPos = cmt.Position
	res.LikeNum = cmt.LikeNum
	res.ReplyNum = cmt.ReplyNum
	res.Creator.StudentID = user.StudentID
	res.Creator.Username = user.Name
	res.Creator.Avatar = user.Avatar
	for _, reply := range replys {
		res.Reply = append(res.Reply, cs.toReply(c, &reply, studentId)) //处理成响应模型，嵌入回复评论一起加载
	}
	return res
}

func (cs *CommentService) toResps(c context.Context, cmts []model.Comment, studentId string) []resp.CommentResp {
	var res []resp.CommentResp
	for _, cmt := range cmts {
		res = append(res, cs.toResp(c, &cmt, studentId))
	}
	return res
}

func (cs *CommentService) toReply(c context.Context, cmt *model.Comment, studentId string) resp.ReplyResp {
	var res resp.ReplyResp                           //返回值
	user, err := cs.ud.GetUserInfo(c, cmt.StudentID) //该条回复用户信息
	if err != nil {
		cs.l.Error("Error get user info when comment to reply", zap.Error(err))
		return resp.ReplyResp{}
	}
	searcher, err := cs.ud.GetUserInfo(c, studentId)
	if err != nil {
		cs.l.Error("Error get user info when comment to reply", zap.Error(err))
		return resp.ReplyResp{}
	}
	pid := cmt.ParentID
	pc := cs.cd.FindCmtByID(c, pid) //父评论
	if pc == nil {
		cs.l.Error("Error find comment by id", zap.String("pid", pid))
		return resp.ReplyResp{}
	}
	pu, err := cs.ud.GetUserInfo(c, pc.StudentID) //父评论用户信息
	if err != nil {
		cs.l.Error("Error get user info when comment to reply", zap.Error(err))
		return resp.ReplyResp{}
	}

	if strings.Contains(searcher.LikeComment, cmt.Bid) {
		res.IsLike = "true"
	} else {
		res.IsLike = "false"
	}
	res.ParentID = cmt.ParentID
	res.RootID = cmt.RootId
	res.ReplyContent = cmt.Content
	res.ReplyTime = tools.ParseTime(cmt.CreatedAt)
	res.Bid = cmt.Bid
	res.ReplyPos = cmt.Position
	res.LikeNum = cmt.LikeNum
	res.ReplyNum = cmt.ReplyNum
	res.ReplyCreator.StudentID = user.StudentID
	res.ReplyCreator.Username = user.Name
	res.ReplyCreator.Avatar = user.Avatar
	res.ParentUserName = pu.Name
	return res
}

func (cs *CommentService) IncreaseCommentNum(c context.Context, parent *ActPostCommentWrapper) error {
	studentId := parent.GetStudentID()
	bid := parent.GetBid()
	switch {
	case parent.Activity != nil:
		return cs.id.CommentActivity(c, studentId, bid)
	case parent.Post != nil:
		return cs.id.CommentPost(c, studentId, bid)
	case parent.Comment != nil:
		return cs.id.CommentComment(c, studentId, bid)
	default:
		return nil
	}
}

package service

import (
	"context"
	"errors"

	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/mq"
	"github.com/raiki02/EG/internal/repo"
	"github.com/raiki02/EG/pkg/encrypt"
	"github.com/raiki02/EG/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var _ CommentServiceHdl = &CommentService{}

type CommentServiceHdl interface {
	CreateComment(c context.Context, cmt *model.Comment, studentID string) (*model.Comment, error)
	DeleteComment(c context.Context, targetID int64, studentID string) error
	AnswerComment(c context.Context, cmt *model.Comment, studentID string) (*model.Comment, error)
	LoadComments(c context.Context, parentID int64) ([]model.Comment, error)
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

// CreateComment 创建一级评论或按 comment 主题创建回复：
// subject=activity/post 时为一级评论，subject=comment 时为对某条评论的回复（写入 ReplyTo 与 root 归属）。
// 写库后自评亦 +1 计数；回复类评论计数失败仅记日志不阻断，避免客户端重试产生重复数据。
func (cs *CommentService) CreateComment(c context.Context, cmt *model.Comment, studentID string) (*model.Comment, error) {
	creator, err := cs.ud.GetUserInfo(c, studentID)
	if err != nil {
		cs.l.Error("Error get user info failed", zap.Error(err), zap.String("studentID", studentID))
		return nil, err
	}
	cmt.CreatorName = model.EncryptedString(creator.Name)
	cmt.CreatorAvatar = creator.Avatar

	var rootID int64
	var rootType string
	if cmt.Subject == SubjectComment {
		parent, err := cs.cd.FindCmtByID(c, cmt.ParentID)
		if err != nil {
			if errors.Is(err, encrypt.ErrDecrypt) {
				cs.l.Error("CreateComment: parent comment decrypt failed", zap.Error(err), zap.Int64("parentId", cmt.ParentID))
				return nil, errs.ErrInternal.Wrap(err)
			}
			return nil, errs.ErrCommentParentNotFound
		}
		if parent == nil {
			return nil, errs.ErrCommentParentNotFound
		}
		// parent 是子级评论时继承其 root_id，保证 root_id 始终指向一级评论
		if parent.Subject == SubjectComment {
			cmt.RootID = parent.RootID
		} else {
			cmt.RootID = parent.Id
		}
		cmt.RootObjectID = parent.RootObjectID
		cmt.RootObjectType = parent.RootObjectType
		// 回复一级评论时不带 ReplyTo（前端用 parentId==rootId 判断直接回复楼主），
		// 回复子级才填被回复人
		if parent.Subject == SubjectComment {
			cmt.ReplyToUserID = parent.StudentID
			cmt.ReplyToUserName = parent.CreatorName
		}
		rootID = parent.RootObjectID
		rootType = parent.RootObjectType
	} else {
		cmt.RootObjectID = cmt.ParentID
		cmt.RootObjectType = cmt.Subject
		rootID = cmt.ParentID
		rootType = cmt.Subject
	}

	subject, err := cs.sg.GetSubjectInfo(c, cmt.ParentID, cmt.Subject)
	if err != nil {
		cs.l.Error("Error get activity or post or comment failed", zap.Error(err))
		return nil, err
	}

	// 计数归属的根对象：subject=comment 时为父评论挂靠的活动/帖子
	rootSubject := subject
	if cmt.Subject == SubjectComment {
		rootSubject, err = cs.sg.GetSubjectInfo(c, rootID, rootType)
		if err != nil {
			cs.l.Error("Error get root subject failed", zap.Error(err))
			return nil, err
		}
	}

	err = cs.cd.CreateComment(c, cmt)
	cs.l.Info("CreateComment",
		zap.Int64("id", cmt.Id),
		zap.String("studentid", cmt.StudentID),
		zap.Int64("parentid", cmt.ParentID),
	)
	if err != nil {
		cs.l.Error("Error comment create failed", zap.Error(err))
		return nil, err
	}

	// 评论已落库，计数失败只记日志不返回错误，避免客户端重试产生重复评论
	switch cmt.Subject {
	case SubjectActivity:
		err = cs.id.CommentActivity(c, studentID, cmt.ParentID)
	case SubjectPost:
		err = cs.id.CommentPost(c, studentID, cmt.ParentID)
	case SubjectComment:
		err = cs.IncreaseCommentNum(c, rootSubject, studentID)
	}
	if err != nil {
		cs.l.Error("Error increase comment num failed",
			zap.Error(err),
			zap.Int64("commentID", cmt.Id),
			zap.String("subject", cmt.Subject),
		)
	}

	if studentID == subject.StudentID {
		return cmt, nil
	}

	// 回复评论对被回复者是 @ 消息，与 AnswerComment 的 Action 保持一致
	action := "comment"
	if cmt.Subject == SubjectComment {
		action = "at"
	}
	f := model.Feed{
		StudentID: studentID,
		TargetId:  cmt.ParentID,
		Object:    cmt.Subject,
		Action:    action,
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

	return cmt, nil
}

// DeleteComment 删除评论：仅限本人。一级评论级联删除整条扁平子树并回减根对象计数，
// 子级评论仅删自身。评论不存在或非本人均幂等返回成功（不通过响应差异泄露存在性）。
func (cs *CommentService) DeleteComment(c context.Context, targetID int64, studentID string) error {
	// 取归属信息用于计数回减与越权判断（只取非加密列，损坏行也允许删除）
	cmt, err := cs.cd.FindCmtByIDNoDecrypt(c, targetID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 评论不存在：幂等成功，不视为错误
			return nil
		}
		cs.l.Error("Error find comment when deleting", zap.Error(err), zap.Int64("targetID", targetID))
		return errs.ErrInternal.Wrap(err)
	}
	if cmt.StudentID != studentID {
		// 非本人：静默 no-op 返回成功，与"不存在"表现一致，避免探测他人评论存在性
		cs.l.Warn("Delete comment by non-owner ignored",
			zap.Int64("commentID", targetID),
			zap.String("operator", studentID),
		)
		return nil
	}

	deleted, err := cs.cd.DeleteCommentCascade(c, studentID, targetID)
	if err != nil {
		cs.l.Error("Error delete comment cascade failed", zap.Error(err), zap.Int64("targetID", targetID))
		return errs.ErrInternal.Wrap(err)
	}
	if deleted == 0 {
		return nil
	}

	// 根对象计数回减：删除了几条就回减几条（一级含子树，子级为 1）
	switch cmt.Subject {
	case SubjectActivity, SubjectPost:
		if cmt.Subject == SubjectActivity {
			if err = cs.id.DecreaseActivityCommentNum(c, cmt.ParentID, deleted); err != nil {
				cs.l.Error("Error decrease activity comment num failed", zap.Error(err))
			}
		} else {
			if err = cs.id.DecreasePostCommentNum(c, cmt.ParentID, deleted); err != nil {
				cs.l.Error("Error decrease post comment num failed", zap.Error(err))
			}
		}
	case SubjectComment:
		// 子级回复：回减它挂靠的活动/帖子（原计数只在 root 归属上 +1）
		if err = cs.decreaseRootCommentNum(c, cmt.RootObjectID, cmt.RootObjectType, deleted); err != nil {
			cs.l.Error("Error decrease root comment num failed", zap.Error(err))
		}
	}
	return nil
}

// decreaseRootCommentNum 按根对象类型回减其评论数；类型未知时静默跳过（历史脏行）。
func (cs *CommentService) decreaseRootCommentNum(c context.Context, rootObjectID int64, rootObjectType string, n int64) error {
	switch rootObjectType {
	case SubjectActivity:
		return cs.id.DecreaseActivityCommentNum(c, rootObjectID, n)
	case SubjectPost:
		return cs.id.DecreasePostCommentNum(c, rootObjectID, n)
	}
	return nil
}

// AnswerComment 回复一条评论（subject 恒为 comment）：root_id 继承父评论、写库前校验根对象存在。
// 写库后计数失败仅记日志不阻断；对被回复者产生 at 消息。
func (cs *CommentService) AnswerComment(c context.Context, cmt *model.Comment, studentID string) (*model.Comment, error) {
	creator, err := cs.ud.GetUserInfo(c, studentID)
	if err != nil {
		cs.l.Error("Error get user info failed", zap.Error(err), zap.String("studentID", studentID))
		return nil, err
	}
	cmt.CreatorName = model.EncryptedString(creator.Name)
	cmt.CreatorAvatar = creator.Avatar

	// 回复评论时 subject 只能是 comment，防止客户端传值导致脏数据
	cmt.Subject = SubjectComment

	parentCmt, err := cs.cd.FindCmtByID(c, cmt.ParentID)
	if err != nil {
		if errors.Is(err, encrypt.ErrDecrypt) {
			cs.l.Error("AnswerComment: parent comment decrypt failed", zap.Error(err), zap.Int64("parentId", cmt.ParentID))
			return nil, errs.ErrInternal.Wrap(err)
		}
		return nil, errs.ErrCommentParentNotFound
	}
	if parentCmt == nil {
		return nil, errs.ErrCommentParentNotFound
	}
	// parent 是子级评论时继承其 root_id，保证 root_id 始终指向一级评论
	if parentCmt.Subject == SubjectComment {
		cmt.RootID = parentCmt.RootID
	} else {
		cmt.RootID = parentCmt.Id
	}
	cmt.RootObjectID = parentCmt.RootObjectID
	cmt.RootObjectType = parentCmt.RootObjectType
	// 回复一级评论时不带 ReplyTo（前端用 parentId==rootId 判断直接回复楼主），
	// 回复子级才填被回复人
	if parentCmt.Subject == SubjectComment {
		cmt.ReplyToUserID = parentCmt.StudentID
		cmt.ReplyToUserName = parentCmt.CreatorName
	}

	rootID := parentCmt.RootObjectID
	rootType := parentCmt.RootObjectType

	// 写库前校验根对象存在，避免接口报错但记录已落库
	root, err := cs.sg.GetSubjectInfo(c, rootID, rootType)
	if err != nil {
		cs.l.Error("Error get root subject failed", zap.Error(err))
		return nil, err
	}

	err = cs.cd.AnswerComment(c, cmt)
	if err != nil {
		cs.l.Error("Error comment answer failed", zap.Error(err))
		return nil, err
	}
	cs.l.Info("AnswerComment",
		zap.Int64("id", cmt.Id),
		zap.String("studentid", cmt.StudentID),
	)

	// 回复已落库，计数失败只记日志不返回错误，避免客户端重试产生重复回复
	if err = cs.IncreaseCommentNum(c, root, studentID); err != nil {
		cs.l.Error("Error increase comment num failed",
			zap.Error(err),
			zap.Int64("commentID", cmt.Id),
			zap.String("subject", cmt.Subject),
		)
	}

	if studentID == parentCmt.StudentID {
		return cmt, nil
	}

	f := model.Feed{
		StudentID: studentID,
		TargetId:  cmt.ParentID,
		Object:    "comment",
		Action:    "at",
		Receiver:  parentCmt.StudentID,
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

func (cs *CommentService) LoadComments(c context.Context, parentID int64) ([]model.Comment, error) {
	cmts, err := cs.cd.LoadComments(c, parentID)
	if err != nil {
		cs.l.Error("Error load comments failed", zap.Error(err))
		return nil, errs.ErrInternal.Wrap(err)
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

	rootIDs := make([]int64, len(cmts))
	for i, cmt := range cmts {
		rootIDs[i] = cmt.Id
	}
	allReplies, err := cs.cd.LoadAnswersBatch(c, rootIDs)
	if err != nil {
		cs.l.Error("Error batch load answers when enriching comments", zap.Error(err))
	}

	replyMap := make(map[int64][]model.Comment)
	for _, reply := range allReplies {
		replyMap[reply.RootID] = append(replyMap[reply.RootID], reply)
		idSet[reply.StudentID] = struct{}{}
	}

	idList := make([]string, 0, len(idSet))
	for id := range idSet {
		idList = append(idList, id)
	}
	userMap, err := cs.ud.GetUsersByIDs(c, idList)
	if err != nil {
		cs.l.Error("Error batch get users", zap.Error(err))
	}

	// 批量查 viewer 对所有评论（含回复）的点赞状态，避免 N+1
	likedMap := make(map[int64]bool)
	commentIds := make([]int64, 0, len(cmts)+len(allReplies))
	for i := range cmts {
		commentIds = append(commentIds, cmts[i].Id)
	}
	for i := range allReplies {
		commentIds = append(commentIds, allReplies[i].Id)
	}
	if viewer, ok := userMap[viewerID]; ok {
		likedMap, err = cs.id.GetUserLikedCommentMap(c, int64(viewer.Id), commentIds)
		if err != nil {
			cs.l.Error("Error batch get comment like status", zap.Error(err))
			likedMap = make(map[int64]bool)
		}
	}

	details := make([]model.CommentDetail, 0, len(cmts))
	for i := range cmts {
		details = append(details, cs.enrichCommentWithCache(c, &cmts[i], viewerID, userMap, replyMap, likedMap))
	}
	return details
}

func (cs *CommentService) EnrichComment(c context.Context, cmt *model.Comment, viewerID string) model.CommentDetail {
	idList := []string{viewerID, cmt.StudentID}
	userMap, _ := cs.ud.GetUsersByIDs(c, idList)
	likedMap := cs.viewerLikedComments(c, viewerID, userMap, []int64{cmt.Id})
	return cs.enrichCommentWithCache(c, cmt, viewerID, userMap, nil, likedMap)
}

func (cs *CommentService) EnrichReply(c context.Context, cmt *model.Comment, viewerID string) model.ReplyDetail {
	idList := []string{viewerID, cmt.StudentID}
	userMap, _ := cs.ud.GetUsersByIDs(c, idList)
	likedMap := cs.viewerLikedComments(c, viewerID, userMap, []int64{cmt.Id})
	return cs.enrichReplyWithCache(c, cmt, viewerID, userMap, likedMap)
}

// viewerLikedComments 批量获取 viewer 对指定评论的点赞状态
func (cs *CommentService) viewerLikedComments(c context.Context, viewerID string, userMap map[string]*model.User, commentIds []int64) map[int64]bool {
	likedMap := make(map[int64]bool)
	if viewer, ok := userMap[viewerID]; ok {
		m, err := cs.id.GetUserLikedCommentMap(c, int64(viewer.Id), commentIds)
		if err != nil {
			cs.l.Error("Error batch get comment like status", zap.Error(err))
			return make(map[int64]bool)
		}
		likedMap = m
	}
	return likedMap
}

func (cs *CommentService) enrichCommentWithCache(c context.Context, cmt *model.Comment, viewerID string, userMap map[string]*model.User, replyMap map[int64][]model.Comment, likedMap map[int64]bool) model.CommentDetail {
	creator := userMap[cmt.StudentID]
	viewer := userMap[viewerID]

	var replies []model.Comment
	if replyMap != nil {
		replies = replyMap[cmt.Id]
	} else {
		var err error
		replies, err = cs.cd.LoadAnswers(c, cmt.Id)
		if err != nil {
			cs.l.Error("Error load answers when enriching comment", zap.Error(err))
		}
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
		detail.IsLike = likedMap[cmt.Id]
	}
	for _, reply := range replies {
		detail.Replies = append(detail.Replies, cs.enrichReplyWithCache(c, &reply, viewerID, userMap, likedMap))
	}
	return detail
}

func (cs *CommentService) enrichReplyWithCache(c context.Context, cmt *model.Comment, viewerID string, userMap map[string]*model.User, likedMap map[int64]bool) model.ReplyDetail {
	isLike := false
	if liked, ok := likedMap[cmt.Id]; ok {
		isLike = liked
	} else if viewer := userMap[viewerID]; viewer != nil {
		// 批量 map 未覆盖（如单条 enrich 路径新加载的回复），回退单条查询
		isLike = cs.id.IsUserLikedComment(c, int64(viewer.Id), cmt.Id)
	}
	return model.ReplyDetail{
		Comment:        *cmt,
		ParentUserName: string(cmt.ReplyToUserName),
		IsLike:         isLike,
	}
}

func (cs *CommentService) IncreaseCommentNum(ctx context.Context, subject SubjectInfo, commenterID string) error {
	switch subject.Subject {
	case SubjectActivity:
		return cs.id.CommentActivity(ctx, commenterID, subject.Id)
	case SubjectPost:
		return cs.id.CommentPost(ctx, commenterID, subject.Id)
	}
	return nil
}

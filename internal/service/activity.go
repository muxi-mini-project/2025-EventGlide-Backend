package service

import (
	"github.com/gin-gonic/gin"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/internal/mq"
	"github.com/raiki02/EG/tools"
	"go.uber.org/zap"
	"strings"
	"time"
)

type ActivityService struct {
	ad  *dao.ActDao
	ch  *cache.Cache
	ud  *dao.UserDao
	id  *dao.InteractionDao
	mq  mq.MQHdl
	aud AuditorService
	l   *zap.Logger
}

func NewActivityService(ad *dao.ActDao, ch *cache.Cache, ud *dao.UserDao, l *zap.Logger, id *dao.InteractionDao, mq mq.MQHdl, aud AuditorService) *ActivityService {
	return &ActivityService{
		ad:  ad,
		ch:  ch,
		ud:  ud,
		id:  id,
		aud: aud,
		mq:  mq,
		l:   l.Named("activity/service"),
	}
}

func (as *ActivityService) NewAct(c *gin.Context, r *req.CreateActReq, studentId string) (resp.CreateActivityResp, error) {
	var (
		form *model.AuditorForm
		err  error
	)
	act := toAct(r, studentId)

	// TODO: 异步增加
	signers := r.LabelForm.Signer
	for _, s := range signers {
		if s.StudentID == studentId {
			continue // 这里避免了 填写了自己的学号名字但是还需要自己审批的情况
		}
		if err := as.id.InsertApprovement(c, s.StudentID, s.Name, act.Bid); err != nil {
			as.l.Error("Failed to insert approvement", zap.Error(err), zap.String("studentID", s.StudentID), zap.String("actBid", act.Bid))
			return resp.CreateActivityResp{}, err
		}
		f := model.Feed{
			StudentId: studentId,
			TargetBid: act.Bid,
			Object:    "activity",
			Action:    "invitation",
			Receiver:  s.StudentID,
		}
		if err := as.mq.Publish(c, "feed_stream", f); err != nil {
			as.l.Error("Failed to publish feed", zap.Error(err), zap.String("studentID", s.StudentID), zap.String("actBid", act.Bid))
			return resp.CreateActivityResp{}, err
		}
	}

	// TODO: 异步
	form, err = as.aud.CreateAuditorForm(c, act.Bid, act.ActiveForm, SubjectActivity)
	if err != nil {
		as.l.Error("Failed to create activity form", zap.Error(err))
		return resp.CreateActivityResp{}, err
	}
	aw := &req.AuditWrapper{
		Subject:   SubjectActivity,
		StudentId: studentId,
		CactReq:   r,
	}

	err = as.aud.UploadForm(c, aw, form.Id)
	if err != nil {
		as.l.Error("Failed to upload form to auditor", zap.Error(err))
		return resp.CreateActivityResp{}, err
	}

	err = as.ad.CreateAct(c, act)
	if err != nil {
		as.l.Error("Failed to create act", zap.Error(err))
		return resp.CreateActivityResp{}, err
	}
	as.l.Info("create activity",
		zap.String("act", act.Bid),
		zap.String("student", act.StudentID),
		zap.String("host", act.HolderType),
		zap.String("formfile", act.ActiveForm),
		zap.String("signer", act.Signer),
	)
	return as.toCreateResp(c, act), nil

}

func (as *ActivityService) NewDraft(c *gin.Context, r *req.CreateActDraftReq, studentId string) (resp.CreateActivityResp, error) {

	d := toActDraft(r, studentId)

	err := as.ad.CreateDraft(c, d)
	if err != nil {
		as.l.Error("Failed to create draft", zap.Error(err))
		return resp.CreateActivityResp{}, err
	}
	as.l.Info("create draft", zap.String("draft", d.Bid), zap.String("student", d.StudentID))

	return as.toCreateResp(c, d), nil
}

func (as *ActivityService) LoadDraft(c *gin.Context, sid string) (model.ActivityDraft, error) {
	d, err := as.ad.LoadDraft(c, sid)
	if err != nil {
		return model.ActivityDraft{}, err
	}

	return d, nil
}

func (as *ActivityService) FindActBySearches(c *gin.Context, req *req.ActSearchReq) ([]resp.ListActivitiesResp, error) {
	acts, err := as.ad.FindActBySearches(c, req)
	if err != nil {
		as.l.Error("Failed to search acts", zap.Error(err))
		return nil, err
	}
	res := as.ToListResp(c, acts)
	return res, nil
}

func (as *ActivityService) FindActByDate(c *gin.Context, date string) ([]resp.ListActivitiesResp, error) {
	acts, err := as.ad.FindActByDate(c, date)
	if err != nil {
		return nil, err
	}
	res := as.ToListResp(c, acts)
	return res, nil
}

func (as *ActivityService) FindActByName(c *gin.Context, name string) ([]resp.ListActivitiesResp, error) {
	acts, err := as.ad.FindActByName(c, name)
	if err != nil {
		return nil, err
	}
	res := as.ToListResp(c, acts)
	return res, nil
}

func (as *ActivityService) FindActByOwnerID(c *gin.Context, id string) ([]resp.ListActivitiesResp, error) {
	acts, err := as.ad.FindActByOwnerID(c, id)
	if err != nil {
		return nil, err
	}
	res := as.ToListResp(c, acts)
	return res, nil
}

func (as *ActivityService) ListAllActs(c *gin.Context, id string) ([]resp.ListActivitiesResp, error) {
	acts, err := as.ad.ListAllActs(c)
	if err != nil {
		return nil, err
	}
	res := as.ToListResp(c, acts)
	return res, nil
}

func (as *ActivityService) ToListResp(c *gin.Context, acts []model.Activity) []resp.ListActivitiesResp {
	var res []resp.ListActivitiesResp
	for _, act := range acts {
		res = append(res, as.toListActResp(c, &act))
	}
	return res
}

func (as *ActivityService) toListActResp(c *gin.Context, act *model.Activity) resp.ListActivitiesResp {

	var res resp.ListActivitiesResp
	sid := tools.GetSid(c)
	searcher := as.ud.FindUserByID(c, sid)
	if strings.Contains(searcher.CollectAct, act.Bid) {
		res.IsCollect = "true"
	} else {
		res.IsCollect = "false"
	}
	if strings.Contains(searcher.LikeAct, act.Bid) {
		res.IsLike = "true"
	} else {
		res.IsLike = "false"
	}
	user := as.ud.FindUserByID(c, act.StudentID)
	res.UserInfo.School = user.School
	res.UserInfo.Username = user.Name
	res.Bid = act.Bid
	res.IsChecking = act.IsChecking
	res.UserInfo.Avatar = user.Avatar
	res.UserInfo.StudentID = user.StudentID
	res.DetailTime.StartTime = act.StartTime
	res.DetailTime.EndTime = act.EndTime
	res.HolderType = act.HolderType
	res.Title = act.Title
	res.Introduce = act.Introduce
	res.Position = act.Position
	res.Type = act.Type
	res.LikeNum = act.LikeNum
	res.CommentNum = act.CommentNum
	res.CollectNum = act.CollectNum
	res.IfRegister = act.IfRegister
	res.ShowImg = tools.StringToSlice(act.ShowImg)

	return res
}

func toAct(r *req.CreateActReq, studentId string) *model.Activity {
	var act model.Activity

	act.Bid = tools.GenUUID()
	act.CreatedAt = time.Now()

	act.StudentID = studentId
	act.Title = r.Title
	act.Introduce = r.Introduce
	act.ShowImg = tools.SliceToString(r.ShowImg)

	act.Position = r.LabelForm.Position
	act.HolderType = r.LabelForm.HolderType
	act.Type = r.LabelForm.Type
	act.IfRegister = r.LabelForm.IfRegister
	act.RegisterMethod = r.LabelForm.RegisterMethod
	act.StartTime = r.LabelForm.StartTime
	act.EndTime = r.LabelForm.EndTime
	act.Signer = tools.SliceToString(joinSigners(r.LabelForm.Signer))
	act.ActiveForm = r.LabelForm.ActiveForm

	return &act
}

func joinSigners(signers []struct {
	StudentID string `json:"studentId" validate:"len=10"`
	Name      string `json:"name"`
}) []string {
	var res []string
	for _, s := range signers {
		s := s.StudentID + ":" + s.Name
		res = append(res, s)
	}
	return res
}

func separateSigners(signers []string) []struct {
	StudentID string `json:"studentId"`
	Name      string `json:"name"`
} {
	var res []struct {
		StudentID string `json:"studentId"`
		Name      string `json:"name"`
	}
	for _, s := range signers {
		ss := strings.Split(s, ":")
		res = append(res, struct {
			StudentID string `json:"studentId"`
			Name      string `json:"name"`
		}{StudentID: ss[0], Name: ss[1]})
	}
	return res
}

func toActDraft(r *req.CreateActDraftReq, studentId string) *model.ActivityDraft {
	var ad model.ActivityDraft
	ad.Bid = tools.GenUUID()
	ad.CreatedAt = time.Now()

	ad.StudentID = studentId
	ad.Title = r.Title
	ad.Introduce = r.Introduce
	ad.ShowImg = tools.SliceToString(r.ShowImg)

	ad.Position = r.LabelForm.Position
	ad.HolderType = r.LabelForm.HolderType
	ad.Type = r.LabelForm.Type
	ad.IfRegister = r.LabelForm.IfRegister
	ad.RegisterMethod = r.LabelForm.RegisterMethod
	ad.StartTime = r.LabelForm.StartTime
	ad.EndTime = r.LabelForm.EndTime
	ad.Signer = tools.SliceToString(joinSigners(r.LabelForm.Signer))
	ad.ActiveForm = r.LabelForm.ActiveForm

	return &ad
}

func (as *ActivityService) toCreateResp(c *gin.Context, act any) resp.CreateActivityResp {
	var res resp.CreateActivityResp

	switch act.(type) {
	case *model.Activity:
		act := act.(*model.Activity)
		user := as.ud.FindUserByID(c, act.StudentID)
		res.Title = act.Title
		res.Introduce = act.Introduce
		res.ShowImg = tools.StringToSlice(act.ShowImg)
		res.Type = act.Type
		res.Bid = act.Bid
		res.ActiveForm = act.ActiveForm
		res.Position = act.Position
		res.IfRegister = act.IfRegister
		res.Signer = separateSigners(tools.StringToSlice(act.Signer))
		res.IsChecking = act.IsChecking
		res.UserInfo.School = user.School
		res.UserInfo.Username = user.Name
		res.UserInfo.Avatar = user.Avatar
		res.UserInfo.StudentID = user.StudentID
		return res

	case *model.ActivityDraft:
		ad := act.(*model.ActivityDraft)
		user := as.ud.FindUserByID(c, ad.StudentID)
		res.Title = ad.Title
		res.Introduce = ad.Introduce
		res.ShowImg = tools.StringToSlice(ad.ShowImg)
		res.Type = ad.Type
		res.Bid = ad.Bid
		res.Position = ad.Position
		res.IfRegister = ad.IfRegister
		res.UserInfo.School = user.School
		res.UserInfo.Username = user.Name
		res.UserInfo.Avatar = user.Avatar
		res.ActiveForm = ad.ActiveForm
		res.Signer = separateSigners(tools.StringToSlice(ad.Signer))
		res.UserInfo.StudentID = user.StudentID
		return res

	case model.ActivityDraft:
		ad := act.(model.ActivityDraft)
		user := as.ud.FindUserByID(c, ad.StudentID)
		res.Title = ad.Title
		res.Introduce = ad.Introduce
		res.ShowImg = tools.StringToSlice(ad.ShowImg)
		res.Type = ad.Type
		res.Position = ad.Position
		res.IfRegister = ad.IfRegister
		res.UserInfo.School = user.School
		res.UserInfo.Username = user.Name
		res.UserInfo.Avatar = user.Avatar
		res.UserInfo.StudentID = user.StudentID
		res.Signer = separateSigners(tools.StringToSlice(ad.Signer))
		return res
	default:
		return res
	}
}

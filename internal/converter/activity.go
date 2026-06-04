package converter

import (
	"strings"
	"time"

	"github.com/muxi-Infra/auditor-Backend/sdk/v2/api/request"
	"github.com/muxi-Infra/auditor-Backend/sdk/v2/dto"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/tools"
)

func CreateActFromReq(r *req.CreateActReq, studentID string) *model.Activity {
	act := &model.Activity{
		Bid:            tools.GenUUID(),
		CreatedAt:      time.Now(),
		StudentID:      studentID,
		Title:          r.Title,
		Introduce:      r.Introduce,
		ShowImg:        tools.SliceToString(r.ShowImg),
		Position:       r.LabelForm.Position,
		HolderType:     r.LabelForm.HolderType,
		Type:           r.LabelForm.Type,
		IfRegister:     r.LabelForm.IfRegister,
		RegisterMethod: r.LabelForm.RegisterMethod,
		StartTime:      r.LabelForm.StartTime,
		EndTime:        r.LabelForm.EndTime,
		ActiveForm:     r.LabelForm.ActiveForm,
		Signers:        SignersFromReqToActivitySigner(r.LabelForm.Signer),
	}
	return act
}

func CreateActDraftFromReq(r *req.CreateActDraftReq, studentID string) *model.ActivityDraft {
	return &model.ActivityDraft{
		Bid:            tools.GenUUID(),
		CreatedAt:      time.Now(),
		StudentID:      studentID,
		Title:          r.Title,
		Introduce:      r.Introduce,
		ShowImg:        tools.SliceToString(r.ShowImg),
		Position:       r.LabelForm.Position,
		HolderType:     r.LabelForm.HolderType,
		Type:           r.LabelForm.Type,
		IfRegister:     r.LabelForm.IfRegister,
		RegisterMethod: r.LabelForm.RegisterMethod,
		StartTime:      r.LabelForm.StartTime,
		EndTime:        r.LabelForm.EndTime,
		ActiveForm:     r.LabelForm.ActiveForm,
		Signers:        SignersFromReqToActivitySigner(r.LabelForm.Signer),
	}
}

func ToLoadDraftResp(d model.ActivityDraft) resp.LoadActivitiesDraftResp {
	var res resp.LoadActivitiesDraftResp

	res.Title = d.Title
	res.Introduce = d.Introduce
	res.ShowImg = tools.StringToSlice(d.ShowImg)

	res.LabelForm.HolderType = d.HolderType
	res.LabelForm.Position = d.Position
	res.LabelForm.IfRegister = d.IfRegister
	res.LabelForm.RegisterMethod = d.RegisterMethod
	res.LabelForm.StartTime = d.StartTime
	res.LabelForm.ActiveForm = d.ActiveForm
	res.LabelForm.EndTime = d.EndTime
	res.LabelForm.Type = d.Type
	res.LabelForm.Signer = ActivitySignersToResp(d.Signers)

	return res
}

func ToListActivitiesResp(details []model.ActivityDetail) []resp.ListActivitiesResp {
	res := make([]resp.ListActivitiesResp, 0, len(details))
	for _, d := range details {
		res = append(res, ToListActivityResp(d))
	}
	return res
}

func ToPaginatedListActivitiesResp(total int64, page, limit int, details []model.ActivityDetail) resp.PaginatedListActivitiesResp {
	return resp.PaginatedListActivitiesResp{
		Total:   total,
		Page:    page,
		Limit:   limit,
		Details: ToListActivitiesResp(details),
	}
}

func ToListActivityResp(d model.ActivityDetail) resp.ListActivitiesResp {
	act := d.Activity
	var res resp.ListActivitiesResp

	if d.IsCollect {
		res.IsCollect = "true"
	} else {
		res.IsCollect = "false"
	}
	if d.IsLike {
		res.IsLike = "true"
	} else {
		res.IsLike = "false"
	}

	res.UserInfo.School = d.Author.School
	res.UserInfo.Username = d.Author.Name
	res.Bid = act.Bid
	res.IsChecking = act.IsChecking
	res.UserInfo.Avatar = d.Author.Avatar
	res.UserInfo.StudentID = d.Author.StudentID
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

func ToCreateActivityResp(d model.ActivityDetail) resp.CreateActivityResp {
	act := d.Activity
	var res resp.CreateActivityResp

	res.Title = act.Title
	res.Introduce = act.Introduce
	res.ShowImg = tools.StringToSlice(act.ShowImg)
	res.Type = act.Type
	res.Bid = act.Bid
	res.ActiveForm = act.ActiveForm
	res.Position = act.Position
	res.IfRegister = act.IfRegister
	res.Signer = ActivitySignersToResp(act.Signers)
	res.IsChecking = act.IsChecking
	res.UserInfo.School = d.Author.School
	res.UserInfo.Username = d.Author.Name
	res.UserInfo.Avatar = d.Author.Avatar
	res.UserInfo.StudentID = d.Author.StudentID

	return res
}

func ToCreateActivityRespFromDraft(d model.ActivityDraft, author model.UserBrief) resp.CreateActivityResp {
	var res resp.CreateActivityResp

	res.Title = d.Title
	res.Introduce = d.Introduce
	res.ShowImg = tools.StringToSlice(d.ShowImg)
	res.Type = d.Type
	res.Bid = d.Bid
	res.Position = d.Position
	res.IfRegister = d.IfRegister
	res.UserInfo.School = author.School
	res.UserInfo.Username = author.Name
	res.UserInfo.Avatar = author.Avatar
	res.ActiveForm = d.ActiveForm
	res.Signer = ActivitySignersToResp(d.Signers)
	res.UserInfo.StudentID = author.StudentID

	return res
}

func SignersFromReq(signers []req.Signer) []model.Signer {
	out := make([]model.Signer, len(signers))
	for i, s := range signers {
		out[i] = model.Signer{StudentID: s.StudentID, Name: s.Name}
	}
	return out
}

func SignersFromReqToActivitySigner(signers []req.Signer) []model.ActivitySigner {
	out := make([]model.ActivitySigner, len(signers))
	for i, s := range signers {
		out[i] = model.ActivitySigner{StudentID: s.StudentID, Name: s.Name}
	}
	return out
}

func ActivitySignersToResp(signers []model.ActivitySigner) []resp.Signer {
	out := make([]resp.Signer, len(signers))
	for i, s := range signers {
		out[i] = resp.Signer{StudentID: s.StudentID, Name: s.Name}
	}
	return out
}

func AuditorUploadReqFromWrapper(aw *req.AuditWrapper, id uint, hookURL string) request.UploadReq {
	now := time.Now().Unix()
	res := request.UploadReq{
		HookUrl:    &hookURL,
		Id:         &id,
		Tags:       &[]string{"校灵通"},
		PublicTime: &now,
	}

	switch aw.Subject {
	case model.SubjectActivity:
		author := extractAuthors(aw.CactReq.LabelForm.Signer)
		res.Author = &author
		*res.Tags = append(*res.Tags, aw.CactReq.LabelForm.Type, "活动")

		ctt := dto.NewContents(
			dto.WithTopicText(aw.CactReq.Title, aw.CactReq.Introduce),
			dto.WithTopicPictures(aw.CactReq.ShowImg),
		)
		res.Content = ctt

		if tools.IfRegisterMapper(aw.CactReq.LabelForm.IfRegister) {
			*res.Tags = append(*res.Tags, "含报名表需要审核")
			res.Content.Topic.Pictures = append(res.Content.Topic.Pictures, aw.CactReq.LabelForm.ActiveForm)
		}
	case model.SubjectPost:
		res.Author = &aw.StudentId
		*res.Tags = append(*res.Tags, "帖子")

		ctt := dto.NewContents(
			dto.WithTopicText(aw.CpostReq.Title, aw.CpostReq.Introduce),
			dto.WithTopicPictures(aw.CpostReq.ShowImg),
		)
		res.Content = ctt
	}

	return res
}

func extractAuthors(signers []req.Signer) string {
	builder := strings.Builder{}
	for _, s := range signers {
		builder.WriteString(s.Name + "-")
	}
	return builder.String()
}

func IndexValid(page, limit int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 10 {
		limit = 10
	}

	return page, limit
}

package converter

import (
	"strings"
	"time"

	"github.com/muxi-Infra/auditor-Backend/sdk/v2/api/request"
	"github.com/muxi-Infra/auditor-Backend/sdk/v2/dto"
	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/pkg/utils"
	"github.com/raiki02/EG/tools"
)

func CreateActFromReq(r *req.CreateActReq, studentID string) *model.Activity {
	id := tools.MustGenerateID()
	act := &model.Activity{
		Id:            id,
		CreatedAt:     time.Now(),
		StudentID:     studentID,
		Title:         r.Title,
		Introduce:     r.Introduce,
		Position:      r.LabelForm.Position,
		HolderType:    r.LabelForm.HolderType,
		Type:          r.LabelForm.Type,
		IfRegister:    r.LabelForm.IfRegister,
		RegisterMethod: r.LabelForm.RegisterMethod,
		StartTime:     r.LabelForm.StartTime,
		EndTime:       r.LabelForm.EndTime,
		ActiveForm:    r.LabelForm.ActiveForm,
		Signers:       SignersFromReqToActivitySigner(r.LabelForm.Signer, id),
		Images:        ImagesFromUrls(r.ShowImg, id, "activity"),
	}
	return act
}

func CreateActDraftFromReq(r *req.CreateActDraftReq, studentID string) *model.ActivityDraft {
	id := tools.MustGenerateID()
	return &model.ActivityDraft{
		Id:            id,
		CreatedAt:     time.Now(),
		StudentID:     studentID,
		Title:         r.Title,
		Introduce:     r.Introduce,
		Position:      r.LabelForm.Position,
		HolderType:    r.LabelForm.HolderType,
		Type:          r.LabelForm.Type,
		IfRegister:    r.LabelForm.IfRegister,
		RegisterMethod: r.LabelForm.RegisterMethod,
		StartTime:     r.LabelForm.StartTime,
		EndTime:       r.LabelForm.EndTime,
		ActiveForm:    r.LabelForm.ActiveForm,
		Signers:       SignersFromReqToActivitySigner(r.LabelForm.Signer, id),
		Images:        ImagesFromUrls(r.ShowImg, id, "activity_draft"),
	}
}

func ImagesFromUrls(urls []string, ownerId int64, ownerType string) []model.Image {
	images := make([]model.Image, 0, len(urls))
	for _, url := range urls {
		id := tools.MustGenerateID()
		images = append(images, model.Image{
			Id:        id,
			OwnerId:   ownerId,
			OwnerType: ownerType,
			Url:       url,
		})
	}
	return images
}

func ToLoadDraftResp(d model.ActivityDraft) resp.LoadActivitiesDraftResp {
	var res resp.LoadActivitiesDraftResp

	res.Title = d.Title
	res.Introduce = d.Introduce
	res.ShowImg = ImagesToUrls(d.Images)

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

func ImagesToUrls(images []model.Image) []string {
	urls := make([]string, len(images))
	for i, img := range images {
		urls[i] = img.Url
	}
	return urls
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
	res.ShowImg = ImagesToUrls(act.Images)
	res.Id = utils.SnowflakeID(act.Id)

	return res
}

func ToCreateActivityResp(d model.ActivityDetail) resp.CreateActivityResp {
	act := d.Activity
	var res resp.CreateActivityResp

	res.Title = act.Title
	res.Introduce = act.Introduce
	res.ShowImg = ImagesToUrls(act.Images)
	res.Type = act.Type
	res.Id = utils.SnowflakeID(act.Id)
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
	res.ShowImg = ImagesToUrls(d.Images)
	res.Type = d.Type
	res.Id = utils.SnowflakeID(d.Id)
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

func SignersFromReqToActivitySigner(signers []req.Signer, activityId int64) []model.ActivitySigner {
	out := make([]model.ActivitySigner, len(signers))
	for i, s := range signers {
		out[i] = model.ActivitySigner{
			Id:         tools.MustGenerateID(),
			ActivityId: activityId,
			StudentID:  s.StudentID,
			Name:       s.Name,
		}
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

func AuditorUploadReqFromWrapper(aw *req.AuditWrapper, id int64, hookURL string) request.UploadReq {
	now := time.Now().Unix()
	idUint := uint(id)
	res := request.UploadReq{
		HookUrl:    &hookURL,
		Id:         &idUint,
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
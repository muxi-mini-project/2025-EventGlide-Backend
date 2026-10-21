package converter

import (
	"errors"
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
		Id:             id,
		CreatedAt:      time.Now(),
		StudentID:      studentID,
		Title:          r.Title,
		Introduce:      r.Introduce,
		OrganizerUnit:  r.LabelForm.OrganizerUnit,
		Position:       r.LabelForm.Position,
		Address:        r.LabelForm.Address,
		HolderType:     r.LabelForm.HolderType,
		Type:           r.LabelForm.Type,
		IfRegister:     r.LabelForm.IfRegister,
		RegisterMethod: r.LabelForm.RegisterMethod,
		StartTime:      r.LabelForm.StartTime,
		EndTime:        r.LabelForm.EndTime,
		ActiveForm:     r.LabelForm.ActiveForm,
		Signers:        SignersFromReqToActivitySigner(r.LabelForm.Signer, id),
		Images:         ImagesFromReq(r.ShowImg, id, "activity"),
	}
	return act
}

func CreateActDraftFromReq(r *req.CreateActDraftReq, studentID string) *model.ActivityDraft {
	id := tools.MustGenerateID()
	return &model.ActivityDraft{
		Id:             id,
		CreatedAt:      time.Now(),
		StudentID:      studentID,
		Title:          r.Title,
		Introduce:      r.Introduce,
		OrganizerUnit:  r.LabelForm.OrganizerUnit,
		Position:       r.LabelForm.Position,
		Address:        r.LabelForm.Address,
		HolderType:     r.LabelForm.HolderType,
		Type:           r.LabelForm.Type,
		IfRegister:     r.LabelForm.IfRegister,
		RegisterMethod: r.LabelForm.RegisterMethod,
		StartTime:      r.LabelForm.StartTime,
		EndTime:        r.LabelForm.EndTime,
		ActiveForm:     r.LabelForm.ActiveForm,
		Signers:        SignersFromReqToActivitySigner(r.LabelForm.Signer, id),
		Images:         ImagesFromReq(r.ShowImg, id, "activity_draft"),
	}
}

// ImagesFromReq 从请求 DTO 构建图片行，宽高随 URL 一起落库，供前端展示占位。
// 客户端值只作提示：宽高齐全才置 ok，否则归零并置 pending，交由回填 worker 异步解析覆盖。
func ImagesFromReq(imgs []req.Image, ownerId int64, ownerType string) []model.Image {
	images := make([]model.Image, 0, len(imgs))
	for _, img := range imgs {
		width, height, status := img.Width, img.Height, model.ImageSizeOK
		if width <= 0 || height <= 0 {
			width, height, status = 0, 0, model.ImageSizePending
		}
		images = append(images, model.Image{
			Id:         tools.MustGenerateID(),
			OwnerId:    ownerId,
			OwnerType:  ownerType,
			Url:        img.Url,
			Width:      width,
			Height:     height,
			SizeStatus: status,
		})
	}
	return images
}

func ToLoadDraftResp(d model.ActivityDraft) resp.LoadActivitiesDraftResp {
	var res resp.LoadActivitiesDraftResp

	res.Title = d.Title
	res.Introduce = d.Introduce
	res.ShowImg = ImagesToResp(d.Images)

	res.LabelForm.HolderType = d.HolderType
	res.LabelForm.OrganizerUnit = d.OrganizerUnit
	res.LabelForm.Position = d.Position
	res.LabelForm.Address = d.Address
	res.LabelForm.IfRegister = d.IfRegister
	res.LabelForm.RegisterMethod = d.RegisterMethod
	res.LabelForm.StartTime = d.StartTime
	res.LabelForm.ActiveForm = d.ActiveForm
	res.LabelForm.EndTime = d.EndTime
	res.LabelForm.Type = d.Type
	res.LabelForm.Signer = ActivitySignersToResp(d.Signers)

	return res
}

// ImagesToResp 将落库图片映射为响应 DTO（含宽高），供各列表/详情/创建响应使用。
func ImagesToResp(images []model.Image) []resp.Image {
	out := make([]resp.Image, len(images))
	for i, img := range images {
		width, height := img.Width, img.Height
		if width < 0 || height < 0 {
			width, height = 0, 0
		}
		out[i] = resp.Image{Url: img.Url, Width: width, Height: height}
	}
	return out
}

// ImagesToReq 将落库图片还原为请求 DTO，供草稿回显与后台送审路径使用。
func ImagesToReq(images []model.Image) []req.Image {
	out := make([]req.Image, len(images))
	for i, img := range images {
		width, height := img.Width, img.Height
		if width < 0 || height < 0 {
			width, height = 0, 0
		}
		out[i] = req.Image{Url: img.Url, Width: width, Height: height}
	}
	return out
}

// ReqImagesToUrls 提取请求 DTO 中的图片 URL，供审核 SDK 等只需 URL 的边界使用。
func ReqImagesToUrls(imgs []req.Image) []string {
	urls := make([]string, len(imgs))
	for i, img := range imgs {
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
	res.OrganizerUnit = act.OrganizerUnit
	res.Position = act.Position
	res.Address = act.Address
	res.Type = act.Type
	res.LikeNum = act.LikeNum
	res.CommentNum = act.CommentNum
	res.CollectNum = act.CollectNum
	res.IfRegister = act.IfRegister
	res.ShowImg = ImagesToResp(act.Images)
	res.Id = utils.SnowflakeID(act.Id)

	return res
}

func ToCreateActivityResp(d model.ActivityDetail) resp.CreateActivityResp {
	act := d.Activity
	var res resp.CreateActivityResp

	res.Title = act.Title
	res.Introduce = act.Introduce
	res.ShowImg = ImagesToResp(act.Images)
	res.Type = act.Type
	res.Id = utils.SnowflakeID(act.Id)
	res.ActiveForm = act.ActiveForm
	res.OrganizerUnit = act.OrganizerUnit
	res.Position = act.Position
	res.Address = act.Address
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
	res.ShowImg = ImagesToResp(d.Images)
	res.Type = d.Type
	res.Id = utils.SnowflakeID(d.Id)
	res.OrganizerUnit = d.OrganizerUnit
	res.Position = d.Position
	res.Address = d.Address
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
		out[i] = model.Signer{StudentID: s.StudentID, Name: truncateName(s.Name)}
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
			Name:       model.EncryptedString(truncateName(s.Name)),
		}
	}
	return out
}

// truncateName 截断姓名到 30 字符，避免加密后超出 varchar(255) 列宽。
// 主路径已有 validator 拦截，此处兜底草稿等无校验路径。
func truncateName(s string) string {
	r := []rune(s)
	if len(r) > 30 {
		return string(r[:30])
	}
	return s
}

func ActivitySignersToResp(signers []model.ActivitySigner) []resp.Signer {
	out := make([]resp.Signer, len(signers))
	for i, s := range signers {
		out[i] = resp.Signer{StudentID: s.StudentID, Name: string(s.Name)}
	}
	return out
}

// ActivityToAuditReq 将已落库的活动还原为审核请求，供后台送审路径使用。
func ActivityToAuditReq(act *model.Activity) *req.CreateActReq {
	signers := make([]req.Signer, 0, len(act.Signers))
	for _, s := range act.Signers {
		signers = append(signers, req.Signer{StudentID: s.StudentID, Name: string(s.Name)})
	}
	return &req.CreateActReq{
		Title:     act.Title,
		Introduce: act.Introduce,
		ShowImg:   ImagesToReq(act.Images),
		LabelForm: req.CreateActLabel{
			HolderType:     act.HolderType,
			OrganizerUnit:  act.OrganizerUnit,
			Position:       act.Position,
			Address:        act.Address,
			IfRegister:     act.IfRegister,
			RegisterMethod: act.RegisterMethod,
			StartTime:      act.StartTime,
			ActiveForm:     act.ActiveForm,
			EndTime:        act.EndTime,
			Type:           act.Type,
			Signer:         signers,
		},
	}
}

func AuditorUploadReqFromWrapper(aw *req.AuditWrapper, id int64, hookURL string) (request.UploadReq, error) {
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
		if aw.CactReq == nil {
			return request.UploadReq{}, errors.New("auditor: activity request is nil")
		}
		author := extractAuthors(aw.CactReq.LabelForm.Signer)
		res.Author = &author
		*res.Tags = append(*res.Tags, aw.CactReq.LabelForm.Type, "活动")

		ctt := dto.NewContents(
			dto.WithTopicText(aw.CactReq.Title, aw.CactReq.Introduce),
			dto.WithTopicPictures(ReqImagesToUrls(aw.CactReq.ShowImg)),
		)
		res.Content = ctt

		if form := aw.CactReq.LabelForm.ActiveForm; form != "" {
			*res.Tags = append(*res.Tags, "含申请表需要审核")
			res.Content.Topic.Pictures = append(res.Content.Topic.Pictures, form)
		}
	case model.SubjectPost:
		if aw.CpostReq == nil {
			return request.UploadReq{}, errors.New("auditor: post request is nil")
		}
		res.Author = &aw.StudentId
		*res.Tags = append(*res.Tags, "帖子")

		ctt := dto.NewContents(
			dto.WithTopicText(aw.CpostReq.Title, aw.CpostReq.Introduce),
			dto.WithTopicPictures(ReqImagesToUrls(aw.CpostReq.ShowImg)),
		)
		res.Content = ctt
	}

	return res, nil
}

func extractAuthors(signers []req.Signer) string {
	builder := strings.Builder{}
	for _, s := range signers {
		builder.WriteString(s.Name + "-")
	}
	return builder.String()
}

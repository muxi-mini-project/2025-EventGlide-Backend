package converter

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/raiki02/EG/api/req"
	"github.com/raiki02/EG/api/resp"
	"github.com/raiki02/EG/internal/model"
	"github.com/raiki02/EG/tools"
)

const legacySignerListSep = ","

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
		Signer:         EncodeSigners(SignersFromReq(r.LabelForm.Signer)),
		ActiveForm:     r.LabelForm.ActiveForm,
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
		Signer:         EncodeSigners(SignersFromReq(r.LabelForm.Signer)),
		ActiveForm:     r.LabelForm.ActiveForm,
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
	res.LabelForm.Signer = SignersToResp(DecodeSigners(d.Signer))

	return res
}

func ToListActivitiesResp(details []model.ActivityDetail) []resp.ListActivitiesResp {
	res := make([]resp.ListActivitiesResp, 0, len(details))
	for _, d := range details {
		res = append(res, ToListActivityResp(d))
	}
	return res
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
	res.Signer = SignersToResp(DecodeSigners(act.Signer))
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
	res.Signer = SignersToResp(DecodeSigners(d.Signer))
	res.UserInfo.StudentID = author.StudentID

	return res
}

func EncodeSigners(signers []model.Signer) string {
	if len(signers) == 0 {
		return ""
	}
	b, err := json.Marshal(signers)
	if err != nil {
		return ""
	}
	return string(b)
}

func DecodeSigners(raw string) []model.Signer {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var signers []model.Signer
		if err := json.Unmarshal([]byte(raw), &signers); err == nil {
			return signers
		}
	}
	return decodeLegacySigners(raw)
}

func decodeLegacySigners(raw string) []model.Signer {
	parts := strings.Split(raw, legacySignerListSep)
	signers := make([]model.Signer, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		studentID, name, ok := strings.Cut(part, ":")
		if !ok || studentID == "" {
			continue
		}
		signers = append(signers, model.Signer{StudentID: studentID, Name: name})
	}
	return signers
}

func SignersFromReq(signers []req.Signer) []model.Signer {
	out := make([]model.Signer, len(signers))
	for i, s := range signers {
		out[i] = model.Signer{StudentID: s.StudentID, Name: s.Name}
	}
	return out
}

func SignersToResp(signers []model.Signer) []resp.Signer {
	out := make([]resp.Signer, len(signers))
	for i, s := range signers {
		out[i] = resp.Signer{StudentID: s.StudentID, Name: s.Name}
	}
	return out
}

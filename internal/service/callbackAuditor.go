package service

import (
	"context"
	"errors"

	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/errs"
	"github.com/raiki02/EG/tools"
)

type CallbackAuditorService interface {
	UpdateStatus(c context.Context, id int64, status string) error
}

type callbackAuditorService struct {
	repo dao.AuditorRepository
}

func (ad *callbackAuditorService) UpdateStatus(c context.Context, id int64, status string) error {
	mapped := tools.StatusMapper(status)
	if mapped == "" {
		return errs.ErrAuditorStatusInvalid.Wrap(errors.New("unknown auditor status: " + status))
	}
	return ad.repo.Update(c, id, mapped)
}

func NewCallbackAuditor(repo dao.AuditorRepository) CallbackAuditorService {
	return &callbackAuditorService{
		repo: repo,
	}
}

package service

import (
	"context"

	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/tools"
)

type CallbackAuditorService interface {
	UpdateStatus(c context.Context, id uint, status string) error
}

type callbackAuditorService struct {
	repo dao.AuditorRepository
}

func (ad *callbackAuditorService) UpdateStatus(c context.Context, id uint, status string) error {
	return ad.repo.Update(c, id, tools.StatusMapper(status))
}

func NewCallbackAuditor(repo dao.AuditorRepository) CallbackAuditorService {
	return &callbackAuditorService{
		repo: repo,
	}
}

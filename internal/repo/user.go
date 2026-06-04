package repo

import (
	"context"

	"github.com/raiki02/EG/internal/cache"
	"github.com/raiki02/EG/internal/dao"
	"github.com/raiki02/EG/internal/model"
)

type UserRepo struct {
	dao *dao.UserDao
	ch  *cache.MultiLevelCache
	kb  cache.KeyBuilder
}

func NewUserRepo(dao *dao.UserDao, ch *cache.MultiLevelCache) *UserRepo {
	return &UserRepo{
		dao: dao,
		ch:  ch,
		kb:  cache.NewKeyBuilder("user"),
	}
}

func (r *UserRepo) Create(ctx context.Context, user *model.User) error {
	if err := r.dao.Create(ctx, user); err != nil {
		return err
	}
	return r.ch.SetAndInvalidate(ctx, r.userInfoKey(user.StudentID), nil, 0)
}

func (r *UserRepo) CheckUserExist(ctx context.Context, studentID string) bool {
	return r.dao.CheckUserExist(ctx, studentID)
}

func (r *UserRepo) GetUserInfo(ctx context.Context, studentID string) (model.User, error) {
	return r.dao.GetUserInfo(ctx, studentID)
}

func (r *UserRepo) GetUsersByIDs(ctx context.Context, ids []string) (map[string]*model.User, error) {
	if len(ids) == 0 {
		return make(map[string]*model.User), nil
	}
	users, err := r.dao.GetUsersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*model.User, len(users))
	for i := range users {
		result[users[i].StudentID] = &users[i]
	}
	return result, nil
}

func (r *UserRepo) FindUserByID(ctx context.Context, studentID string) model.User {
	user, err := r.GetUserInfo(ctx, studentID)
	if err != nil {
		return model.User{}
	}
	return user
}

func (r *UserRepo) UpdateAvatar(ctx context.Context, studentID, imgURL string) error {
	if err := r.dao.UpdateAvatar(ctx, studentID, imgURL); err != nil {
		return err
	}
	return r.ch.SetAndInvalidate(ctx, r.userInfoKey(studentID), nil, 0)
}

func (r *UserRepo) UpdateUsername(ctx context.Context, studentID, name string) error {
	if err := r.dao.UpdateUsername(ctx, studentID, name); err != nil {
		return err
	}
	return r.ch.SetAndInvalidate(ctx, r.userInfoKey(studentID), nil, 0)
}

func (r *UserRepo) UpdateCollege(ctx context.Context, studentID string, college string) error {
	if err := r.dao.UpdateCollege(ctx, studentID, college); err != nil {
		return err
	}

	return r.ch.SetAndInvalidate(ctx, r.userInfoKey(studentID), nil, 0)
}

func (r *UserRepo) UpdateRealName(ctx context.Context, studentID string, realName string) error {
	if err := r.dao.UpdateRealName(ctx, studentID, realName); err != nil {
		return err
	}
	return r.ch.SetAndInvalidate(ctx, r.userInfoKey(studentID), nil, 0)
}

func (r *UserRepo) Invalidate(ctx context.Context, studentID string) error {
	return r.ch.SetAndInvalidate(ctx, r.userInfoKey(studentID), nil, 0)
}

func (r *UserRepo) userInfoKey(studentID string) string {
	return r.kb.Build("info", studentID)
}

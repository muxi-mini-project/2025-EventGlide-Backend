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
	//return cache.GetTyped(r.ch, ctx, r.userInfoKey(studentID), 10*time.Minute, func(context.Context) (model.User, error) {
	//	user, err := r.dao.GetUserInfo(ctx, studentID)
	//	if err != nil {
	//		if errors.Is(err, gorm.ErrRecordNotFound) {
	//			return model.User{}, cache.MarkNotFound(err)
	//		}
	//		return model.User{}, err
	//	}
	//	return user, nil
	//})
	return r.dao.GetUserInfo(ctx, studentID)
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

func (r *UserRepo) Invalidate(ctx context.Context, studentID string) error {
	return r.ch.SetAndInvalidate(ctx, r.userInfoKey(studentID), nil, 0)
}

func (r *UserRepo) userInfoKey(studentID string) string {
	return r.kb.Build("info", studentID)
}

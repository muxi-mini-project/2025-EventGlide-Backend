package dao

import (
	"context"

	"github.com/raiki02/EG/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type UserDaoHdl interface {
	UpdateAvatar(context.Context, string, string) error
	UpdateUsername(context.Context, string, string) error
	Create(context.Context, *model.User) error
	CheckUserExist(context.Context, string) bool
	GetUserInfo(context.Context, string) (model.User, error)
	FindUserByID(context.Context, string) model.User
}

type UserDao struct {
	db *gorm.DB
	l  *zap.Logger
}

func NewUserDao(db *gorm.DB, l *zap.Logger) *UserDao {
	return &UserDao{
		db: db,
		l:  l.Named("user/dao"),
	}
}

func (ud *UserDao) UpdateAvatar(ctx context.Context, student_id string, imgurl string) error {
	return ud.db.WithContext(ctx).Model(&model.User{}).Where("student_id = ?", student_id).Update("avatar", imgurl).Error
}

func (ud *UserDao) UpdateUsername(ctx context.Context, student_id string, name string) error {
	return ud.db.WithContext(ctx).Model(&model.User{}).Where("student_id = ?", student_id).Update("name", name).Error
}

func (ud *UserDao) Create(ctx context.Context, user *model.User) error {
	return ud.db.WithContext(ctx).Create(user).Error
}

func (ud *UserDao) CheckUserExist(ctx context.Context, student_id string) bool {
	res := ud.db.WithContext(ctx).Where("student_id = ?", student_id).Find(&model.User{}).RowsAffected
	return res != 0
}

func (ud *UserDao) GetUserInfo(ctx context.Context, student_id string) (model.User, error) {
	var user model.User
	err := ud.db.WithContext(ctx).Where("student_id = ?", student_id).First(&user).Error
	return user, err
}

func (ud *UserDao) FindUserByID(ctx context.Context, student_id string) model.User {
	var user model.User
	err := ud.db.WithContext(ctx).Where("student_id = ?", student_id).First(&user).Error
	if err != nil {
		return model.User{}
	}
	return user
}

package service

import (
	"context"

	"github.com/qiniu/go-sdk/v7/auth"
	"github.com/qiniu/go-sdk/v7/storage"
	"github.com/raiki02/EG/config"
)

type ImgUploaderHdl interface {
	GetQIQIUToken(context.Context) string
}

type ImgUploader struct {
	AccessKey string
	SerectKey string
	Bucket    string
	ImgUrl    string
}

func NewImgUploader(cfg *config.Conf) *ImgUploader {
	img := &ImgUploader{
		AccessKey: cfg.Imgbed.AccessKey,
		SerectKey: cfg.Imgbed.SecretKey,
		Bucket:    cfg.Imgbed.Bucket,
		ImgUrl:    cfg.Imgbed.ImgURL,
	}
	return img
}

func (iu *ImgUploader) GenQINIUToken(c context.Context) string {
	mac := auth.New(iu.AccessKey, iu.SerectKey)
	putPolicy := storage.PutPolicy{
		Scope:   iu.Bucket,
		Expires: 3600,
	}
	upToken := putPolicy.UploadToken(mac)
	return upToken
}

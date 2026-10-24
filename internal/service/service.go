package service

import "github.com/google/wire"

var Provider = wire.NewSet(
	NewActivityService,
	NewAuditorUploadWorker,
	NewCCNUService,
	NewCommentService,
	NewAuditorService,
	NewFeedService,
	NewCallbackAuditor,
	NewImageSizeResolver,
	NewImageSizeWorker,
	NewImgUploader,
	NewPostService,
	NewInteractionService,
	NewUserService,
	wire.Bind(new(StudentCrawler), new(*ccnuService)),

	NewSubjectGetter,
)

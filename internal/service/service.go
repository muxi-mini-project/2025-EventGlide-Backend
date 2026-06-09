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
	NewImgUploader,
	NewPostService,
	NewInteractionService,
	NewUserService,

	NewSubjectGetter,
)

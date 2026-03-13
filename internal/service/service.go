package service

import "github.com/google/wire"

var Provider = wire.NewSet(
	NewActivityService,
	NewCCNUService,
	NewCommentService,
	NewAuditorService,
	NewFeedService,
	NewCallbackAuditor,
	NewImgUploader,
	NewPostService,
	NewInteractionService,
	NewUserService,

	NewActPostCommentGetter,
)

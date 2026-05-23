package handler

import "github.com/google/wire"

var Provider = wire.NewSet(
	NewActHandler,
	NewCommentHandler,
	NewFeedHandler,
	NewInteractionHandler,
	NewPostHandler,
	NewUserHandler,
	NewCallbackAuditorHandler,
	NewHandler,
)

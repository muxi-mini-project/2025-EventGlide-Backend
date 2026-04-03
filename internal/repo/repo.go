package repo

import "github.com/google/wire"

var Provider = wire.NewSet(
	NewUserRepo,
	NewActivityRepo,
	NewPostRepo,
	NewInteractionRepo,
)

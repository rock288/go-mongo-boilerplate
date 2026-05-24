package user

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewUserRepository,
	NewUserService,
	NewUserHandler,
	// feature:kafka:start
	NewEventHandler,
	// feature:kafka:end
)

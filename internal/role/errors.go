package role

import "errors"

var (
	ErrRoleNotFound      = errors.New("role not found")
	ErrRoleAlreadyExists = errors.New("role already exists")
	ErrInvalidRoleID     = errors.New("invalid role id")
)

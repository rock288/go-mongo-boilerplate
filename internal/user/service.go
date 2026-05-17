package user

import (
	"context"

	"github.com/google/uuid"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/pagination"
)

type UserService interface {
	Register(ctx context.Context, req RegisterRequest) (*User, error)
	List(ctx context.Context, cursor string, limit int) (pagination.Page[UserResponse], error)
}

type userService struct {
	repo UserRepository
}

func NewUserService(repo UserRepository) UserService {
	return &userService{repo: repo}
}

func (s *userService) Register(ctx context.Context, req RegisterRequest) (*User, error) {
	exists, err := s.repo.ExistsByEmail(ctx, req.Email)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrUserAlreadyExists
	}

	u := &User{
		UUID:     uuid.New(),
		UserName: req.UserName,
		Email:    req.Email,
		IsActive: true,
	}
	if err := s.repo.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// List returns a page of users keyed by an opaque cursor. limit <= 0 yields
// the default page size; limit > MaxLimit is clamped.
func (s *userService) List(ctx context.Context, cursor string, limit int) (pagination.Page[UserResponse], error) {
	cur, err := pagination.DecodeCursor(cursor)
	if err != nil {
		return pagination.Page[UserResponse]{}, err
	}
	limit = pagination.ClampLimit(limit)

	users, err := s.repo.List(ctx, cur, limit)
	if err != nil {
		return pagination.Page[UserResponse]{}, err
	}

	hasMore := len(users) > limit
	if hasMore {
		users = users[:limit]
	}

	items := make([]UserResponse, len(users))
	for i, u := range users {
		items[i] = UserResponse{ID: u.ID.Hex(), Email: u.Email, UserName: u.UserName}
	}

	page := pagination.Page[UserResponse]{Items: items, HasMore: hasMore}
	if hasMore && len(users) > 0 {
		page.NextCursor = pagination.Cursor{LastID: users[len(users)-1].ID}.Encode()
	}
	return page, nil
}

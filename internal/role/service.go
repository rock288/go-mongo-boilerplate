package role

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type RoleService interface {
	Create(ctx context.Context, req CreateRoleRequest) (*Role, error)
	GetByID(ctx context.Context, id string) (*Role, error)
	List(ctx context.Context) ([]*Role, error)
}

type roleService struct {
	repo RoleRepository
}

func NewRoleService(repo RoleRepository) RoleService {
	return &roleService{repo: repo}
}

func (s *roleService) Create(ctx context.Context, req CreateRoleRequest) (*Role, error) {
	exists, err := s.repo.ExistsByName(ctx, req.Name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrRoleAlreadyExists
	}

	r := &Role{
		Name:        req.Name,
		Description: req.Description,
	}
	if err := s.repo.Create(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *roleService) GetByID(ctx context.Context, id string) (*Role, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, ErrInvalidRoleID
	}
	return s.repo.FindByID(ctx, oid)
}

func (s *roleService) List(ctx context.Context) ([]*Role, error) {
	return s.repo.List(ctx)
}

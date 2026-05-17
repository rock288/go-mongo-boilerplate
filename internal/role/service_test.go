package role_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/rock288/go-mongo-boilerplate/internal/role"
	"github.com/rock288/go-mongo-boilerplate/internal/role/mocks"
)

func TestRoleService_Create_Success(t *testing.T) {
	repo := mocks.NewRoleRepository(t)
	repo.EXPECT().ExistsByName(mock.Anything, "admin").Return(false, nil)
	repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(r *role.Role) bool {
		return r.Name == "admin" && r.Description == "root"
	})).Return(nil)

	svc := role.NewRoleService(repo)
	r, err := svc.Create(context.Background(), role.CreateRoleRequest{
		Name:        "admin",
		Description: "root",
	})

	assert.NoError(t, err)
	assert.Equal(t, "admin", r.Name)
}

func TestRoleService_Create_AlreadyExists(t *testing.T) {
	repo := mocks.NewRoleRepository(t)
	repo.EXPECT().ExistsByName(mock.Anything, "admin").Return(true, nil)

	svc := role.NewRoleService(repo)
	_, err := svc.Create(context.Background(), role.CreateRoleRequest{Name: "admin"})

	assert.ErrorIs(t, err, role.ErrRoleAlreadyExists)
}

func TestRoleService_GetByID_InvalidHex(t *testing.T) {
	repo := mocks.NewRoleRepository(t)
	svc := role.NewRoleService(repo)

	_, err := svc.GetByID(context.Background(), "not-a-valid-objectid")
	assert.ErrorIs(t, err, role.ErrInvalidRoleID)
}

func TestRoleService_GetByID_NotFound(t *testing.T) {
	oid := bson.NewObjectID()
	repo := mocks.NewRoleRepository(t)
	repo.EXPECT().FindByID(mock.Anything, oid).Return(nil, role.ErrRoleNotFound)

	svc := role.NewRoleService(repo)
	_, err := svc.GetByID(context.Background(), oid.Hex())
	assert.ErrorIs(t, err, role.ErrRoleNotFound)
}

func TestRoleService_GetByID_RepoError(t *testing.T) {
	oid := bson.NewObjectID()
	repo := mocks.NewRoleRepository(t)
	repo.EXPECT().FindByID(mock.Anything, oid).Return(nil, errors.New("boom"))

	svc := role.NewRoleService(repo)
	_, err := svc.GetByID(context.Background(), oid.Hex())
	assert.Error(t, err)
	assert.NotErrorIs(t, err, role.ErrRoleNotFound)
}

func TestRoleService_List_Empty(t *testing.T) {
	repo := mocks.NewRoleRepository(t)
	repo.EXPECT().List(mock.Anything).Return([]*role.Role{}, nil)

	svc := role.NewRoleService(repo)
	out, err := svc.List(context.Background())
	assert.NoError(t, err)
	assert.Empty(t, out)
}

func TestRoleService_List_Multi(t *testing.T) {
	repo := mocks.NewRoleRepository(t)
	want := []*role.Role{{Name: "admin"}, {Name: "user"}}
	repo.EXPECT().List(mock.Anything).Return(want, nil)

	svc := role.NewRoleService(repo)
	got, err := svc.List(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

package role_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/rock288/go-mongo-boilerplate/internal/role"
	"github.com/rock288/go-mongo-boilerplate/internal/role/mocks"
	"github.com/rock288/go-mongo-boilerplate/internal/testutil"
)

func TestRoleHandler_Create_Success(t *testing.T) {
	t.Parallel()

	svc := mocks.NewRoleService(t)
	created := &role.Role{ID: bson.NewObjectID(), Name: "admin", Description: "elevated"}
	svc.On("Create", mock.Anything, role.CreateRoleRequest{
		Name: "admin", Description: "elevated",
	}).Return(created, nil).Once()

	h := role.NewRoleHandler(svc)
	c, w := testutil.NewTestContext(t, http.MethodPost, "/roles", role.CreateRoleRequest{
		Name: "admin", Description: "elevated",
	})
	h.Create(c)

	require.Equal(t, http.StatusCreated, w.Code)
	var resp role.RoleResponse
	testutil.DecodeJSON(t, w, &resp)
	assert.Equal(t, created.ID.Hex(), resp.ID)
	assert.Equal(t, "admin", resp.Name)
}

func TestRoleHandler_Create_DuplicateName_Returns409(t *testing.T) {
	t.Parallel()

	svc := mocks.NewRoleService(t)
	svc.On("Create", mock.Anything, mock.Anything).Return(nil, role.ErrRoleAlreadyExists).Once()

	h := role.NewRoleHandler(svc)
	c, w := testutil.NewTestContext(t, http.MethodPost, "/roles", role.CreateRoleRequest{Name: "admin"})
	h.Create(c)

	require.Equal(t, http.StatusConflict, w.Code)
}

func TestRoleHandler_Create_InvalidInput_Returns400(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body any
	}{
		{"missing_name", role.CreateRoleRequest{Description: "x"}},
		{"name_too_short", role.CreateRoleRequest{Name: "a"}},
		{"malformed_json", "{bad"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := mocks.NewRoleService(t)
			h := role.NewRoleHandler(svc)
			c, w := testutil.NewTestContext(t, http.MethodPost, "/roles", tt.body)
			h.Create(c)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestRoleHandler_Create_ServiceError_Returns500(t *testing.T) {
	t.Parallel()

	svc := mocks.NewRoleService(t)
	svc.On("Create", mock.Anything, mock.Anything).Return(nil, errors.New("repo failure")).Once()

	h := role.NewRoleHandler(svc)
	c, w := testutil.NewTestContext(t, http.MethodPost, "/roles", role.CreateRoleRequest{Name: "admin"})
	h.Create(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestRoleHandler_GetByID_Success(t *testing.T) {
	t.Parallel()

	id := bson.NewObjectID()
	svc := mocks.NewRoleService(t)
	svc.On("GetByID", mock.Anything, id.Hex()).
		Return(&role.Role{ID: id, Name: "admin"}, nil).Once()

	h := role.NewRoleHandler(svc)
	c, w := testutil.NewTestContext(t, http.MethodGet, "/roles/"+id.Hex(), nil)
	c.Params = []gin.Param{{Key: "id", Value: id.Hex()}}
	h.GetByID(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp role.RoleResponse
	testutil.DecodeJSON(t, w, &resp)
	assert.Equal(t, id.Hex(), resp.ID)
}

func TestRoleHandler_GetByID_InvalidID_Returns400(t *testing.T) {
	t.Parallel()

	svc := mocks.NewRoleService(t)
	svc.On("GetByID", mock.Anything, "not-an-oid").Return(nil, role.ErrInvalidRoleID).Once()

	h := role.NewRoleHandler(svc)
	c, w := testutil.NewTestContext(t, http.MethodGet, "/roles/bad", nil)
	c.Params = []gin.Param{{Key: "id", Value: "not-an-oid"}}
	h.GetByID(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRoleHandler_GetByID_NotFound_Returns404(t *testing.T) {
	t.Parallel()

	id := bson.NewObjectID().Hex()
	svc := mocks.NewRoleService(t)
	svc.On("GetByID", mock.Anything, id).Return(nil, role.ErrRoleNotFound).Once()

	h := role.NewRoleHandler(svc)
	c, w := testutil.NewTestContext(t, http.MethodGet, "/roles/"+id, nil)
	c.Params = []gin.Param{{Key: "id", Value: id}}
	h.GetByID(c)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestRoleHandler_GetByID_ServiceError_Returns500(t *testing.T) {
	t.Parallel()

	id := bson.NewObjectID().Hex()
	svc := mocks.NewRoleService(t)
	svc.On("GetByID", mock.Anything, id).Return(nil, errors.New("driver err")).Once()

	h := role.NewRoleHandler(svc)
	c, w := testutil.NewTestContext(t, http.MethodGet, "/roles/"+id, nil)
	c.Params = []gin.Param{{Key: "id", Value: id}}
	h.GetByID(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestRoleHandler_List_Success(t *testing.T) {
	t.Parallel()

	svc := mocks.NewRoleService(t)
	svc.On("List", mock.Anything).Return([]*role.Role{
		{ID: bson.NewObjectID(), Name: "admin"},
		{ID: bson.NewObjectID(), Name: "user"},
	}, nil).Once()

	h := role.NewRoleHandler(svc)
	c, w := testutil.NewTestContext(t, http.MethodGet, "/roles", nil)
	h.List(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp []role.RoleResponse
	testutil.DecodeJSON(t, w, &resp)
	assert.Len(t, resp, 2)
}

func TestRoleHandler_List_Empty_Returns200EmptyArray(t *testing.T) {
	t.Parallel()

	svc := mocks.NewRoleService(t)
	svc.On("List", mock.Anything).Return([]*role.Role{}, nil).Once()

	h := role.NewRoleHandler(svc)
	c, w := testutil.NewTestContext(t, http.MethodGet, "/roles", nil)
	h.List(c)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[]", w.Body.String())
}

func TestRoleHandler_List_ServiceError_Returns500(t *testing.T) {
	t.Parallel()

	svc := mocks.NewRoleService(t)
	svc.On("List", mock.Anything).Return(nil, errors.New("db down")).Once()

	h := role.NewRoleHandler(svc)
	c, w := testutil.NewTestContext(t, http.MethodGet, "/roles", nil)
	h.List(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

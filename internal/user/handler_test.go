package user_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/pagination"
	"github.com/rock288/go-mongo-boilerplate/internal/testutil"
	"github.com/rock288/go-mongo-boilerplate/internal/user"
	"github.com/rock288/go-mongo-boilerplate/internal/user/mocks"
)

func TestUserHandler_Register_Success(t *testing.T) {
	t.Parallel()

	svc := mocks.NewUserService(t)
	expectedID := bson.NewObjectID()
	svc.On("Register", mock.Anything, user.RegisterRequest{
		Email:    "a@b.c",
		UserName: "alice",
		Purpose:  "demo",
	}).Return(&user.User{ID: expectedID, Email: "a@b.c"}, nil).Once()

	h := user.NewUserHandler(svc)
	c, w := testutil.NewTestContext(t, http.MethodPost, "/users", user.RegisterRequest{
		Email: "a@b.c", UserName: "alice", Purpose: "demo",
	})
	h.Register(c)

	require.Equal(t, http.StatusCreated, w.Code)
	var resp user.RegisterResponse
	testutil.DecodeJSON(t, w, &resp)
	assert.Equal(t, expectedID.Hex(), resp.ID)
	assert.Equal(t, "a@b.c", resp.Email)
}

func TestUserHandler_Register_DuplicateEmail_Returns409(t *testing.T) {
	t.Parallel()

	svc := mocks.NewUserService(t)
	svc.On("Register", mock.Anything, mock.Anything).Return(nil, user.ErrUserAlreadyExists).Once()

	h := user.NewUserHandler(svc)
	c, w := testutil.NewTestContext(t, http.MethodPost, "/users", user.RegisterRequest{
		Email: "dup@b.c", UserName: "dup", Purpose: "demo",
	})
	h.Register(c)

	require.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "user already exists")
}

func TestUserHandler_Register_InvalidEmail_Returns400(t *testing.T) {
	t.Parallel()

	svc := mocks.NewUserService(t) // no calls expected
	h := user.NewUserHandler(svc)

	c, w := testutil.NewTestContext(t, http.MethodPost, "/users", user.RegisterRequest{
		Email: "not-an-email", UserName: "alice", Purpose: "demo",
	})
	h.Register(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUserHandler_Register_MissingFields_Returns400(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body user.RegisterRequest
	}{
		{"missing_email", user.RegisterRequest{UserName: "alice", Purpose: "demo"}},
		{"missing_username", user.RegisterRequest{Email: "a@b.c", Purpose: "demo"}},
		{"missing_purpose", user.RegisterRequest{Email: "a@b.c", UserName: "alice"}},
		{"username_too_short", user.RegisterRequest{Email: "a@b.c", UserName: "ab", Purpose: "demo"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := mocks.NewUserService(t)
			h := user.NewUserHandler(svc)
			c, w := testutil.NewTestContext(t, http.MethodPost, "/users", tt.body)
			h.Register(c)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestUserHandler_Register_MalformedJSON_Returns400(t *testing.T) {
	t.Parallel()

	svc := mocks.NewUserService(t)
	h := user.NewUserHandler(svc)
	c, w := testutil.NewTestContext(t, http.MethodPost, "/users", "{not-json")
	h.Register(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUserHandler_Register_ServiceError_Returns500(t *testing.T) {
	t.Parallel()

	svc := mocks.NewUserService(t)
	svc.On("Register", mock.Anything, mock.Anything).
		Return(nil, errors.New("db unreachable")).Once()

	h := user.NewUserHandler(svc)
	c, w := testutil.NewTestContext(t, http.MethodPost, "/users", user.RegisterRequest{
		Email: "a@b.c", UserName: "alice", Purpose: "demo",
	})
	h.Register(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "db unreachable")
}

func TestUserHandler_List_Success_ReturnsPage(t *testing.T) {
	t.Parallel()

	expected := pagination.Page[user.UserResponse]{
		Items:      []user.UserResponse{{ID: "abc", Email: "a@b", UserName: "alice"}},
		NextCursor: "next",
		HasMore:    true,
	}
	svc := mocks.NewUserService(t)
	svc.On("List", mock.Anything, "cur", 50).Return(expected, nil).Once()

	h := user.NewUserHandler(svc)
	c, w := testutil.NewTestContext(t, http.MethodGet, "/users?cursor=cur&limit=50", nil)
	h.List(c)

	require.Equal(t, http.StatusOK, w.Code)
	var got pagination.Page[user.UserResponse]
	testutil.DecodeJSON(t, w, &got)
	assert.Equal(t, expected, got)
}

func TestUserHandler_List_DefaultsWhenQueryAbsent(t *testing.T) {
	t.Parallel()

	svc := mocks.NewUserService(t)
	svc.On("List", mock.Anything, "", 0).
		Return(pagination.Page[user.UserResponse]{Items: []user.UserResponse{}}, nil).Once()

	h := user.NewUserHandler(svc)
	c, w := testutil.NewTestContext(t, http.MethodGet, "/users", nil)
	h.List(c)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestUserHandler_List_ServiceError_Returns400(t *testing.T) {
	t.Parallel()

	svc := mocks.NewUserService(t)
	svc.On("List", mock.Anything, "bad", 0).
		Return(pagination.Page[user.UserResponse]{}, errors.New("invalid cursor encoding")).Once()

	h := user.NewUserHandler(svc)
	c, w := testutil.NewTestContext(t, http.MethodGet, "/users?cursor=bad", nil)
	h.List(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid cursor")
}

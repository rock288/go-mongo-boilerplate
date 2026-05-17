package user_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/pagination"
	"github.com/rock288/go-mongo-boilerplate/internal/user"
	"github.com/rock288/go-mongo-boilerplate/internal/user/mocks"
)

func TestUserService_Register_Success(t *testing.T) {
	repo := mocks.NewUserRepository(t)
	repo.EXPECT().ExistsByEmail(mock.Anything, "a@b.com").Return(false, nil)
	repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(u *user.User) bool {
		return u.Email == "a@b.com" && u.UserName == "alice" && u.IsActive
	})).Return(nil)

	svc := user.NewUserService(repo)
	u, err := svc.Register(context.Background(), user.RegisterRequest{
		Email:    "a@b.com",
		UserName: "alice",
		Purpose:  "test",
	})

	assert.NoError(t, err)
	assert.Equal(t, "a@b.com", u.Email)
	assert.True(t, u.IsActive)
	assert.NotZero(t, u.UUID)
}

func TestUserService_Register_AlreadyExists(t *testing.T) {
	repo := mocks.NewUserRepository(t)
	repo.EXPECT().ExistsByEmail(mock.Anything, "a@b.com").Return(true, nil)

	svc := user.NewUserService(repo)
	_, err := svc.Register(context.Background(), user.RegisterRequest{
		Email:    "a@b.com",
		UserName: "alice",
		Purpose:  "test",
	})

	assert.ErrorIs(t, err, user.ErrUserAlreadyExists)
}

func TestUserService_Register_ExistsCheckFails(t *testing.T) {
	repo := mocks.NewUserRepository(t)
	repo.EXPECT().ExistsByEmail(mock.Anything, "a@b.com").Return(false, errors.New("db down"))

	svc := user.NewUserService(repo)
	_, err := svc.Register(context.Background(), user.RegisterRequest{
		Email:    "a@b.com",
		UserName: "alice",
		Purpose:  "test",
	})

	assert.Error(t, err)
	assert.NotErrorIs(t, err, user.ErrUserAlreadyExists)
}

func TestUserService_Register_CreateFails(t *testing.T) {
	repo := mocks.NewUserRepository(t)
	repo.EXPECT().ExistsByEmail(mock.Anything, "a@b.com").Return(false, nil)
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("insert failed"))

	svc := user.NewUserService(repo)
	_, err := svc.Register(context.Background(), user.RegisterRequest{
		Email:    "a@b.com",
		UserName: "alice",
		Purpose:  "test",
	})

	assert.Error(t, err)
}

func TestUserService_List_NoMore(t *testing.T) {
	t.Parallel()

	repo := mocks.NewUserRepository(t)
	id1 := bson.NewObjectID()
	repo.EXPECT().
		List(mock.Anything, pagination.Cursor{}, 20).
		Return([]*user.User{{ID: id1, Email: "a@b", UserName: "alice"}}, nil)

	svc := user.NewUserService(repo)
	page, err := svc.List(context.Background(), "", 0)

	require.NoError(t, err)
	assert.False(t, page.HasMore)
	assert.Empty(t, page.NextCursor)
	assert.Len(t, page.Items, 1)
	assert.Equal(t, id1.Hex(), page.Items[0].ID)
}

func TestUserService_List_HasMore_EmitsCursor(t *testing.T) {
	t.Parallel()

	repo := mocks.NewUserRepository(t)
	// limit=2 → repo asked for 3, returns 3 → page returns 2 items + cursor
	id1, id2, id3 := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	repo.EXPECT().
		List(mock.Anything, pagination.Cursor{}, 2).
		Return([]*user.User{
			{ID: id1, Email: "1@x", UserName: "u1"},
			{ID: id2, Email: "2@x", UserName: "u2"},
			{ID: id3, Email: "3@x", UserName: "u3"},
		}, nil)

	svc := user.NewUserService(repo)
	page, err := svc.List(context.Background(), "", 2)

	require.NoError(t, err)
	assert.True(t, page.HasMore)
	assert.Len(t, page.Items, 2)
	require.NotEmpty(t, page.NextCursor)

	// cursor must decode back to id2 (last emitted item)
	cur, err := pagination.DecodeCursor(page.NextCursor)
	require.NoError(t, err)
	assert.Equal(t, id2, cur.LastID)
}

func TestUserService_List_InvalidCursor_ReturnsError(t *testing.T) {
	t.Parallel()

	repo := mocks.NewUserRepository(t) // repo not called
	svc := user.NewUserService(repo)

	_, err := svc.List(context.Background(), "!!!not-base64!!!", 0)
	assert.Error(t, err)
}

func TestUserService_List_RepoError_PropagatesError(t *testing.T) {
	t.Parallel()

	repo := mocks.NewUserRepository(t)
	repo.EXPECT().
		List(mock.Anything, pagination.Cursor{}, 20).
		Return(nil, errors.New("mongo down"))

	svc := user.NewUserService(repo)
	_, err := svc.List(context.Background(), "", 0)
	assert.Error(t, err)
}

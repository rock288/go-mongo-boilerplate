package user

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/pagination"
)

type UserRepository interface {
	FindByEmail(ctx context.Context, email string) (*User, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	Create(ctx context.Context, u *User) error
	List(ctx context.Context, cur pagination.Cursor, limit int) ([]*User, error)
}

type userRepository struct {
	coll *mongo.Collection
}

func NewUserRepository(db *mongo.Database) UserRepository {
	return &userRepository{coll: db.Collection(Collection)}
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := r.coll.FindOne(ctx, bson.M{"email": email}).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	count, err := r.coll.CountDocuments(ctx, bson.M{"email": email})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *userRepository) Create(ctx context.Context, u *User) error {
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now
	res, err := r.coll.InsertOne(ctx, u)
	if err != nil {
		return err
	}
	if oid, ok := res.InsertedID.(bson.ObjectID); ok {
		u.ID = oid
	}
	return nil
}

// List returns up to limit+1 users with _id strictly greater than cur.LastID,
// sorted ascending by _id. The +1 row lets the caller detect HasMore without
// a separate count query.
func (r *userRepository) List(ctx context.Context, cur pagination.Cursor, limit int) ([]*User, error) {
	filter := bson.M{}
	if !cur.LastID.IsZero() {
		filter["_id"] = bson.M{"$gt": cur.LastID}
	}
	opts := options.Find().SetSort(bson.M{"_id": 1}).SetLimit(int64(limit + 1))
	c, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close(ctx) }()

	users := make([]*User, 0, limit+1)
	if err := c.All(ctx, &users); err != nil {
		return nil, err
	}
	return users, nil
}

package role

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type RoleRepository interface {
	Create(ctx context.Context, r *Role) error
	FindByID(ctx context.Context, id bson.ObjectID) (*Role, error)
	FindByName(ctx context.Context, name string) (*Role, error)
	List(ctx context.Context) ([]*Role, error)
	ExistsByName(ctx context.Context, name string) (bool, error)
}

type roleRepository struct {
	coll *mongo.Collection
}

func NewRoleRepository(db *mongo.Database) RoleRepository {
	return &roleRepository{coll: db.Collection(Collection)}
}

func (r *roleRepository) Create(ctx context.Context, role *Role) error {
	now := time.Now().UTC()
	role.CreatedAt = now
	role.UpdatedAt = now
	res, err := r.coll.InsertOne(ctx, role)
	if err != nil {
		return err
	}
	if oid, ok := res.InsertedID.(bson.ObjectID); ok {
		role.ID = oid
	}
	return nil
}

func (r *roleRepository) FindByID(ctx context.Context, id bson.ObjectID) (*Role, error) {
	var role Role
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&role)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *roleRepository) FindByName(ctx context.Context, name string) (*Role, error) {
	var role Role
	err := r.coll.FindOne(ctx, bson.M{"name": name}).Decode(&role)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *roleRepository) List(ctx context.Context) ([]*Role, error) {
	cur, err := r.coll.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()

	roles := make([]*Role, 0)
	if err := cur.All(ctx, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

func (r *roleRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	count, err := r.coll.CountDocuments(ctx, bson.M{"name": name})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

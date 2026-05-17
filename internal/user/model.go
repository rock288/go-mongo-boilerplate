package user

import (
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const Collection = "users"

type User struct {
	ID        bson.ObjectID   `bson:"_id,omitempty"      json:"id"`
	UUID      uuid.UUID       `bson:"uuid"               json:"uuid"`
	UserName  string          `bson:"user_name"          json:"user_name"`
	Email     string          `bson:"email"              json:"email"`
	IsActive  bool            `bson:"is_active"          json:"is_active"`
	RoleIDs   []bson.ObjectID `bson:"role_ids,omitempty" json:"role_ids,omitempty"`
	CreatedAt time.Time       `bson:"created_at"         json:"created_at"`
	UpdatedAt time.Time       `bson:"updated_at"         json:"updated_at"`
}

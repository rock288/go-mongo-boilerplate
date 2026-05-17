// Package pagination provides cursor-based pagination primitives for Mongo
// collections. Cursors are opaque base64(JSON) tokens — NOT signed. If client
// tampering is a concern in your service, wrap them with HMAC before exposing.
package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	// MaxLimit caps any client-requested page size.
	MaxLimit = 100
	// DefaultLimit is applied when the client sends limit <= 0.
	DefaultLimit = 20
)

// Cursor carries the position of the previous page. Only LastID is needed for
// `_id`-sorted pagination, which is the default for Mongo collections.
type Cursor struct {
	LastID bson.ObjectID `json:"last_id"`
}

// Encode serialises the cursor to a URL-safe base64 string.
func (c Cursor) Encode() string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor parses a string produced by Encode. An empty input returns a
// zero Cursor with no error — useful for the first page.
func DecodeCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, errors.New("invalid cursor encoding")
	}
	var c Cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return Cursor{}, errors.New("invalid cursor payload")
	}
	return c, nil
}

// Page is the canonical list-response envelope. Use as a generic over the
// caller's transport DTO (e.g. Page[UserResponse]).
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

// ClampLimit enforces [1, MaxLimit] with DefaultLimit as the fallback.
func ClampLimit(req int) int {
	if req <= 0 {
		return DefaultLimit
	}
	if req > MaxLimit {
		return MaxLimit
	}
	return req
}

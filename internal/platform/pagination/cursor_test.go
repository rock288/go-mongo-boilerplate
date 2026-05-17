package pagination_test

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/pagination"
)

func TestCursor_EncodeDecode_Roundtrip(t *testing.T) {
	t.Parallel()

	id := bson.NewObjectID()
	encoded := pagination.Cursor{LastID: id}.Encode()
	assert.NotEmpty(t, encoded)

	got, err := pagination.DecodeCursor(encoded)
	require.NoError(t, err)
	assert.Equal(t, id, got.LastID)
}

func TestDecodeCursor_Empty_ReturnsZeroNoError(t *testing.T) {
	t.Parallel()

	got, err := pagination.DecodeCursor("")
	require.NoError(t, err)
	assert.True(t, got.LastID.IsZero())
}

func TestDecodeCursor_GarbageBase64_ReturnsError(t *testing.T) {
	t.Parallel()

	_, err := pagination.DecodeCursor("!!!not-base64!!!")
	assert.Error(t, err)
}

func TestDecodeCursor_ValidBase64InvalidJSON_ReturnsError(t *testing.T) {
	t.Parallel()

	bad := base64.RawURLEncoding.EncodeToString([]byte("{not json"))
	_, err := pagination.DecodeCursor(bad)
	assert.Error(t, err)
}

func TestClampLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   int
		want int
	}{
		{"zero_uses_default", 0, pagination.DefaultLimit},
		{"negative_uses_default", -1, pagination.DefaultLimit},
		{"in_range", 50, 50},
		{"above_max_clamps", 101, pagination.MaxLimit},
		{"max_passes_through", pagination.MaxLimit, pagination.MaxLimit},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, pagination.ClampLimit(tt.in))
		})
	}
}

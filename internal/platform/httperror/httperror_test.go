package httperror_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/httperror"
)

func init() { gin.SetMode(gin.TestMode) }

func newCtx() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	return c, w
}

func TestEnvelope_ShapeAndStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		call     func(c *gin.Context)
		wantCode string
		wantStat int
	}{
		{"bad_request", func(c *gin.Context) { httperror.BadRequest(c, errors.New("bad")) }, "BAD_REQUEST", http.StatusBadRequest},
		{"conflict", func(c *gin.Context) { httperror.Conflict(c, errors.New("dup")) }, "CONFLICT", http.StatusConflict},
		{"not_found", func(c *gin.Context) { httperror.NotFound(c, errors.New("gone")) }, "NOT_FOUND", http.StatusNotFound},
		{"internal", func(c *gin.Context) { httperror.Internal(c, errors.New("boom")) }, "INTERNAL", http.StatusInternalServerError},
	}
	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, w := newCtx()
			tt.call(c)
			require.Equal(t, tt.wantStat, w.Code)

			var env httperror.Envelope
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
			assert.Equal(t, tt.wantCode, env.Error.Code)
			assert.NotEmpty(t, env.Error.Message)
		})
	}
}

func TestWrite_OmitsCodeWhenEmpty(t *testing.T) {
	t.Parallel()

	c, w := newCtx()
	httperror.Write(c, http.StatusTeapot, "", "no code here")

	require.Equal(t, http.StatusTeapot, w.Code)
	// JSON should not contain a "code" key when empty.
	assert.NotContains(t, w.Body.String(), `"code"`)
	assert.Contains(t, w.Body.String(), `"message":"no code here"`)
}

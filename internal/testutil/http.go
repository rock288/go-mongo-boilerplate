// Package testutil contains shared helpers for unit, integration, and E2E tests.
package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// NewTestContext builds a gin.Context backed by a fresh httptest.ResponseRecorder
// and a synthetic *http.Request. body may be nil, a string, or any JSON-encodable value.
// Returned recorder captures the response for assertions.
func NewTestContext(t testing.TB, method, path string, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var reader io.Reader
	switch b := body.(type) {
	case nil:
		reader = nil
	case string:
		reader = strings.NewReader(b)
	case []byte:
		reader = bytes.NewReader(b)
	default:
		buf, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequest(method, path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.Request = req
	return c, w
}

// DecodeJSON unmarshals the recorder body into v. Fails the test on error.
func DecodeJSON(t testing.TB, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("decode JSON %q: %v", w.Body.String(), err)
	}
}

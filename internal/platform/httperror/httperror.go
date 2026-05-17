// Package httperror provides a shared JSON error envelope and convenience
// helpers so handlers don't redefine `gin.H{"error": err.Error()}` everywhere.
//
// Envelope shape:
//
//	{
//	  "error": {
//	    "code":    "USER_ALREADY_EXISTS",   // optional, machine-readable
//	    "message": "user already exists"     // human-readable
//	  }
//	}
//
// The helpers write the response and abort the handler chain. Pass an `err`
// for development convenience — the message comes from err.Error(). For
// stable contracts in your own service, pass a literal string or a sentinel.
package httperror

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Envelope is the canonical error response body.
type Envelope struct {
	Error Body `json:"error"`
}

// Body holds the machine code and human message.
type Body struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// Write emits {error: {code, message}} with the given HTTP status and aborts.
// If code is empty, only message is sent.
func Write(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, Envelope{Error: Body{Code: code, Message: message}})
}

// WriteErr is a convenience wrapper that pulls the message from err.Error().
// Use Write directly when you want a static message.
func WriteErr(c *gin.Context, status int, code string, err error) {
	Write(c, status, code, err.Error())
}

// BadRequest writes a 400 envelope. Use for malformed input or invalid query.
func BadRequest(c *gin.Context, err error) {
	WriteErr(c, http.StatusBadRequest, "BAD_REQUEST", err)
}

// Conflict writes a 409 envelope. Use for duplicate-key / already-exists.
func Conflict(c *gin.Context, err error) {
	WriteErr(c, http.StatusConflict, "CONFLICT", err)
}

// NotFound writes a 404 envelope.
func NotFound(c *gin.Context, err error) {
	WriteErr(c, http.StatusNotFound, "NOT_FOUND", err)
}

// Internal writes a 500 envelope. Caller is responsible for logging the
// underlying error before calling — the response body still echoes err.Error()
// for development. Replace with a redacted message in production if needed.
func Internal(c *gin.Context, err error) {
	WriteErr(c, http.StatusInternalServerError, "INTERNAL", err)
}

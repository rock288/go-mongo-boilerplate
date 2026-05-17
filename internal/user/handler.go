package user

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/httperror"
)

type UserHandler struct {
	svc UserService
}

func NewUserHandler(svc UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

func (h *UserHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httperror.BadRequest(c, err)
		return
	}

	u, err := h.svc.Register(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrUserAlreadyExists):
			httperror.Conflict(c, err)
		default:
			httperror.Internal(c, err)
		}
		return
	}

	c.JSON(http.StatusCreated, RegisterResponse{
		ID:    u.ID.Hex(),
		Email: u.Email,
	})
}

// List returns a page of users. Query params: cursor (opaque string),
// limit (1..MaxLimit, default 20).
func (h *UserHandler) List(c *gin.Context) {
	cursor := c.Query("cursor")
	limit, _ := strconv.Atoi(c.Query("limit")) // 0 on parse error → ClampLimit handles

	page, err := h.svc.List(c.Request.Context(), cursor, limit)
	if err != nil {
		httperror.BadRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

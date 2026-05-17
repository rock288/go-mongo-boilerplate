package role

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/httperror"
)

type RoleHandler struct {
	svc RoleService
}

func NewRoleHandler(svc RoleService) *RoleHandler {
	return &RoleHandler{svc: svc}
}

func (h *RoleHandler) Create(c *gin.Context) {
	var req CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httperror.BadRequest(c, err)
		return
	}

	r, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, ErrRoleAlreadyExists) {
			httperror.Conflict(c, err)
			return
		}
		httperror.Internal(c, err)
		return
	}
	c.JSON(http.StatusCreated, toResponse(r))
}

func (h *RoleHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	r, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidRoleID):
			httperror.BadRequest(c, err)
		case errors.Is(err, ErrRoleNotFound):
			httperror.NotFound(c, err)
		default:
			httperror.Internal(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, toResponse(r))
}

func (h *RoleHandler) List(c *gin.Context) {
	roles, err := h.svc.List(c.Request.Context())
	if err != nil {
		httperror.Internal(c, err)
		return
	}

	out := make([]RoleResponse, 0, len(roles))
	for _, r := range roles {
		out = append(out, toResponse(r))
	}
	c.JSON(http.StatusOK, out)
}

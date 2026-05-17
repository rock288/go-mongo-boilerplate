package role

import "github.com/gin-gonic/gin"

func RegisterRoutes(rg *gin.RouterGroup, h *RoleHandler) {
	roles := rg.Group("/roles")
	roles.POST("", h.Create)
	roles.GET("", h.List)
	roles.GET("/:id", h.GetByID)
}

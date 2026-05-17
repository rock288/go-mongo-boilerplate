package user

import "github.com/gin-gonic/gin"

func RegisterRoutes(rg *gin.RouterGroup, h *UserHandler) {
	users := rg.Group("/users")
	users.POST("/register", h.Register)
	users.GET("", h.List)
}

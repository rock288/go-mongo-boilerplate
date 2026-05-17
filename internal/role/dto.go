package role

type CreateRoleRequest struct {
	Name        string `json:"name"                  binding:"required,min=2"`
	Description string `json:"description,omitempty"`
}

type RoleResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func toResponse(r *Role) RoleResponse {
	return RoleResponse{
		ID:          r.ID.Hex(),
		Name:        r.Name,
		Description: r.Description,
	}
}

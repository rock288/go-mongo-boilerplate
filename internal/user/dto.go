package user

type RegisterRequest struct {
	Email    string `json:"email"     binding:"required,email"`
	UserName string `json:"user_name" binding:"required,min=3"`
	Purpose  string `json:"purpose"   binding:"required"`
}

type RegisterResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// UserResponse is the public list/detail representation of a User.
type UserResponse struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	UserName string `json:"user_name"`
}

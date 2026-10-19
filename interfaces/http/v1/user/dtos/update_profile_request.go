package dtos

type UpdateProfileRequest struct {
	Username  *string `json:"username,omitempty"`
	AvatarUrl *string `json:"avatar_url,omitempty"`
}

type UpdateProfileResponse struct {
	Id        string `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	AvatarUrl string `json:"avatar_url"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required,min=8"`
	NewPassword     string `json:"new_password" validate:"required,min=8"`
}

type ChangePasswordResponse struct {
	Message string `json:"message"`
}

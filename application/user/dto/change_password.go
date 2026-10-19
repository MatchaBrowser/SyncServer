package dto

type ChangePasswordInput struct {
	UserId          string
	CurrentPassword string
	NewPassword     string
}

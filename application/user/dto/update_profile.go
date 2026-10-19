package dto

import "github.com/matchabrowser/syncserver/domain"

type UpdateProfileInput struct {
	UserId    string
	Username  *string
	AvatarUrl *string
}

type UpdateProfileOutput struct {
	User *domain.User
}

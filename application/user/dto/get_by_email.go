package dto

import "github.com/matchabrowser/syncserver/domain"

type GetByEmailInput struct {
	Email string
}

type GetByEmailOutput struct {
	User *domain.User
}

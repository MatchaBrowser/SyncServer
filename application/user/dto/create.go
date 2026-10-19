package dto

import "github.com/matchabrowser/syncserver/domain"

type CreateUserInput struct {
	User domain.User
}

type CreateUserOutput struct {
	User *domain.User
}

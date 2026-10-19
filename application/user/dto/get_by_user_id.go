package dto

import "github.com/matchabrowser/syncserver/domain"

type GetByUserIdInput struct {
	UserId string
}

type GetByUserIdOutput struct {
	User *domain.User
}

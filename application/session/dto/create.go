package dto

import "time"

type CreateSessionInput struct {
	UserId    string
	UserAgent string
	IpAddress string
	ExpiresAt time.Time
}

type CreateSessionOutput struct {
	SessionId string
}

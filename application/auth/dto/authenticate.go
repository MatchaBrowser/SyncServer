package dto

import "github.com/gofiber/fiber/v3"

type AuthenticateInput struct {
	Email    string
	Password string
	Context  fiber.Ctx
}

type AuthenticateOutput struct {
	Token string
}

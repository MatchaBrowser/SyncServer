package dto

import "github.com/gofiber/fiber/v3"

type SSORedirectOutput struct {
	URL   string
	State string
}

type SSOCallbackInput struct {
	Code    string
	State   string
	Context fiber.Ctx
}

type SSOCallbackOutput struct {
	Token string
}

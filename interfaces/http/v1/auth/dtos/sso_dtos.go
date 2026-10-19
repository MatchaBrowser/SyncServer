package dtos

type SSORedirectResponse struct {
	URL   string `json:"url"`
	State string `json:"state"`
}

type SSOCallbackRequest struct {
	Code  string `json:"code" validate:"required"`
	State string `json:"state" validate:"required"`
}

type SSOCallbackResponse struct {
	Token string `json:"token"`
}

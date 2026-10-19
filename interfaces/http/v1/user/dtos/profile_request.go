package dtos

type ProfileResponse struct {
	Id           string         `json:"id"`
	Username     string         `json:"username"`
	Email        string         `json:"email"`
	Avatar       string         `json:"avatar"`
	Role         string         `json:"role"`
	Preferences  map[string]any `json:"preferences,omitempty"`
	SsoProviders []string       `json:"sso_providers,omitempty"`
}

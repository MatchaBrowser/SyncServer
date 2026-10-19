package server

import (
	"github.com/matchabrowser/syncserver/interfaces/http/v1/dtos"

	"github.com/gofiber/fiber/v3"
	fiberoapi "github.com/labbs/fiber-oapi/v3"
)

type Limits struct {
	MaxPayloadBytes   int  `json:"maxPayloadBytes"`
	MaxChangesPerPush int  `json:"maxChangesPerPush"`
	MaxRequestBytes   int  `json:"maxRequestBytes"`
	MaxBlobBytes      int  `json:"maxBlobBytes"`
	MaxAccountBytes   *int `json:"maxAccountBytes"`
}

type ServerResponse struct {
	API               int      `json:"api"`
	Software          string   `json:"software"`
	Registration      string   `json:"registration"`
	Limits            Limits   `json:"limits"`
	MinimumIterations int      `json:"minimumIterations"`
	MaximumIterations int      `json:"maximumIterations"`
	Features          []string `json:"features"`
}

func (ctrl Controller) GetServer(ctx fiber.Ctx, input dtos.Empty) (*ServerResponse, *fiberoapi.Problem) {
	cfg := ctrl.Config
	limits := Limits{
		MaxPayloadBytes:   cfg.Sync.MaxPayloadBytes,
		MaxChangesPerPush: cfg.Sync.MaxChangesPerPush,
		MaxRequestBytes:   cfg.Sync.MaxRequestBytes,
		MaxBlobBytes:      cfg.Sync.MaxBlobBytes,
	}
	if quota := cfg.Sync.MaxAccountBytes; quota > 0 {
		limits.MaxAccountBytes = &quota
	}
	features := ctrl.Features
	if features == nil {
		features = []string{}
	}
	return &ServerResponse{
		API:               1,
		Software:          "syncserver " + cfg.Version,
		Registration:      cfg.Registration.Mode,
		Limits:            limits,
		MinimumIterations: cfg.Kdf.MinimumIterations,
		MaximumIterations: cfg.Kdf.MaximumIterations,
		Features:          features,
	}, nil
}

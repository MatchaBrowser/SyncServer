package devices

import (
	"strings"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/interfaces/http/authn"
	"github.com/matchabrowser/syncserver/interfaces/http/problem"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/dtos"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	fiberoapi "github.com/labbs/fiber-oapi/v3"
	"github.com/rs/zerolog"
)

type Device struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	CreatedAt  domain.Timestamp `json:"createdAt"`
	LastSeenAt domain.Timestamp `json:"lastSeenAt"`
	Current    bool             `json:"current"`
}

type DevicesResponse struct {
	Devices []Device `json:"devices"`
}

type DeviceRequest struct {
	ID string `uri:"id" validate:"required,uuid"`
}

type RenameDeviceRequest struct {
	ID   string `uri:"id" validate:"required,uuid"`
	Name string `json:"name" validate:"required,max=100"`
}

func toDevice(d domain.Device, currentID string) Device {
	return Device{ID: d.ID, Name: d.Name, CreatedAt: domain.Timestamp(d.CreatedAt), LastSeenAt: domain.Timestamp(d.LastSeenAt), Current: d.ID == currentID}
}

func (ctrl Controller) logger(ctx fiber.Ctx, component string) zerolog.Logger {
	return ctrl.Logger.With().Str("request_id", requestid.FromContext(ctx)).Str("component", "http.api.v1.devices."+component).Logger()
}

func (ctrl Controller) ListDevices(ctx fiber.Ctx, input dtos.Empty) (*DevicesResponse, *fiberoapi.Problem) {
	principal, ok := authn.FromContext(ctx)
	if !ok {
		return nil, problem.New(problem.InvalidToken, "")
	}
	devices, err := ctrl.AccountApplication.ListDevices(ctx.Context(), principal.AccountID)
	if err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "list"), err)
	}
	response := &DevicesResponse{Devices: make([]Device, 0, len(devices))}
	for _, d := range devices {
		response.Devices = append(response.Devices, toDevice(d, principal.DeviceID))
	}
	return response, nil
}

func (ctrl Controller) RenameDevice(ctx fiber.Ctx, req RenameDeviceRequest) (*Device, *fiberoapi.Problem) {
	principal, ok := authn.FromContext(ctx)
	if !ok {
		return nil, problem.New(problem.InvalidToken, "")
	}
	device, err := ctrl.AccountApplication.RenameDevice(ctx.Context(), principal.AccountID, strings.ToLower(req.ID), strings.TrimSpace(req.Name))
	if err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "rename"), err)
	}
	d := toDevice(*device, principal.DeviceID)
	return &d, nil
}

func (ctrl Controller) RevokeDevice(ctx fiber.Ctx, req DeviceRequest) (*dtos.Empty, *fiberoapi.Problem) {
	principal, ok := authn.FromContext(ctx)
	if !ok {
		return nil, problem.New(problem.InvalidToken, "")
	}
	if err := ctrl.AccountApplication.RevokeDevice(ctx.Context(), principal.AccountID, strings.ToLower(req.ID)); err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "revoke"), err)
	}
	return &dtos.Empty{}, nil
}

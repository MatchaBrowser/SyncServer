package sync

import (
	"crypto/sha256"

	"github.com/matchabrowser/syncserver/application/records"
	"github.com/matchabrowser/syncserver/interfaces/http/authn"
	"github.com/matchabrowser/syncserver/interfaces/http/problem"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	fiberoapi "github.com/labbs/fiber-oapi/v3"
	"github.com/rs/zerolog"
)

type ChangesRequest struct {
	Since string `query:"since" validate:"omitempty,max=64" description:"Cursor returned by the previous read; absent for a first read"`
	Limit int    `query:"limit" validate:"omitempty,min=1" description:"Most records returned: 500 by default, 1000 at most"`
}

type PushRequest struct {
	IdempotencyKey string       `header:"Idempotency-Key" validate:"required,uuid"`
	Changes        []PushChange `json:"changes" validate:"required,dive"`
}

type PushChange struct {
	ID          string `json:"id" validate:"required,uuid"`
	BaseVersion *int64 `json:"baseVersion"`
	Deleted     bool   `json:"deleted"`
	KeyVersion  int    `json:"keyVersion" validate:"required,min=1"`
	// Payload is decoded by the application: a malformed one rejects its
	// change only (invalid_payload).
	Payload string `json:"payload" validate:"required"`
}

func (ctrl Controller) logger(ctx fiber.Ctx, component string) zerolog.Logger {
	return ctrl.Logger.With().Str("request_id", requestid.FromContext(ctx)).Str("component", "http.api.v1.sync."+component).Logger()
}

func (ctrl Controller) Changes(ctx fiber.Ctx, req ChangesRequest) (*records.ChangesOutput, *fiberoapi.Problem) {
	principal, ok := authn.FromContext(ctx)
	if !ok {
		return nil, problem.New(problem.InvalidToken, "")
	}
	output, err := ctrl.RecordsApplication.Changes(ctx.Context(), principal.AccountID, req.Since, req.Limit)
	if err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "changes"), err)
	}
	return output, nil
}

func (ctrl Controller) Push(ctx fiber.Ctx, req PushRequest) (*records.PushOutput, *fiberoapi.Problem) {
	principal, ok := authn.FromContext(ctx)
	if !ok {
		return nil, problem.New(problem.InvalidToken, "")
	}
	changes := make([]records.Change, len(req.Changes))
	for i, c := range req.Changes {
		changes[i] = records.Change{ID: c.ID, BaseVersion: c.BaseVersion, Deleted: c.Deleted, KeyVersion: c.KeyVersion, Payload: c.Payload}
	}
	// The same key must come back with the same body.
	bodyHash := sha256.Sum256(ctx.Body())
	output, err := ctrl.RecordsApplication.Push(ctx.Context(), principal.AccountID, principal.DeviceID, req.IdempotencyKey, bodyHash[:], changes)
	if err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "push"), err)
	}
	return output, nil
}

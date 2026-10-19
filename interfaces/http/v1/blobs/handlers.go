package blobs

import (
	"io"

	"github.com/matchabrowser/syncserver/interfaces/http/authn"
	"github.com/matchabrowser/syncserver/interfaces/http/problem"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/dtos"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	fiberoapi "github.com/labbs/fiber-oapi/v3"
	"github.com/rs/zerolog"
)

type BlobRequest struct {
	ID string `uri:"id" validate:"required,uuid"`
}

type PutBlobRequest struct {
	ID          string `uri:"id" validate:"required,uuid"`
	IfNoneMatch string `header:"If-None-Match" validate:"required,eq=*"`
	Body        []byte `body:"raw" validate:"required"`
}

// PutBlobResponse is 201 for a new package, 200 for the same content sent
// again.
type PutBlobResponse struct {
	created bool
}

func (r PutBlobResponse) HTTPStatus() int {
	if r.created {
		return fiber.StatusCreated
	}
	return fiber.StatusOK
}

// blobStream sends a package from its file, without loading it in memory.
type blobStream struct {
	file io.ReadCloser
	size int64
}

func (s *blobStream) Respond(c fiber.Ctx) error {
	c.Set(fiber.HeaderContentType, fiber.MIMEOctetStream)
	// fasthttp closes the file once it is sent.
	return c.SendStream(s.file, int(s.size))
}

func (ctrl Controller) logger(ctx fiber.Ctx, component string) zerolog.Logger {
	return ctrl.Logger.With().Str("request_id", requestid.FromContext(ctx)).Str("component", "http.api.v1.blobs."+component).Logger()
}

func (ctrl Controller) PutBlob(ctx fiber.Ctx, req PutBlobRequest) (*PutBlobResponse, *fiberoapi.Problem) {
	principal, ok := authn.FromContext(ctx)
	if !ok {
		return nil, problem.New(problem.InvalidToken, "")
	}
	created, err := ctrl.BlobsApplication.Put(ctx.Context(), principal.AccountID, req.ID, req.Body)
	if err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "put"), err)
	}
	return &PutBlobResponse{created: created}, nil
}

func (ctrl Controller) GetBlob(ctx fiber.Ctx, req BlobRequest) (*blobStream, *fiberoapi.Problem) {
	principal, ok := authn.FromContext(ctx)
	if !ok {
		return nil, problem.New(problem.InvalidToken, "")
	}
	file, size, err := ctrl.BlobsApplication.Get(ctx.Context(), principal.AccountID, req.ID)
	if err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "get"), err)
	}
	return &blobStream{file: file, size: size}, nil
}

func (ctrl Controller) DeleteBlob(ctx fiber.Ctx, req BlobRequest) (*dtos.Empty, *fiberoapi.Problem) {
	principal, ok := authn.FromContext(ctx)
	if !ok {
		return nil, problem.New(problem.InvalidToken, "")
	}
	if err := ctrl.BlobsApplication.Delete(ctx.Context(), principal.AccountID, req.ID); err != nil {
		return nil, problem.FromError(ctrl.logger(ctx, "delete"), err)
	}
	return &dtos.Empty{}, nil
}

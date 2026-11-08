package http

import (
	"errors"
	"io"
	"net/http"

	"github.com/vbloher/canton-private-otc-desk/pkg/render"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/dto"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/usecase"
)

const maxWebhookBody = 1 << 20

func newWebhook(uc *usecase.UseCase) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
		if err != nil {
			render.JSON(w, render.Err{Error: "invalid request"}, http.StatusBadRequest)

			return
		}

		err = uc.HandleResendWebhook(r.Context(), dto.ResendWebhookInput{
			Payload:   payload,
			ID:        r.Header.Get("Svix-Id"),
			Timestamp: r.Header.Get("Svix-Timestamp"),
			Signature: r.Header.Get("Svix-Signature"),
		})

		switch {
		case err == nil:
			w.WriteHeader(http.StatusNoContent)
		case errors.Is(err, domain.ErrUnauthorized):
			_ = render.Error(r.Context(), err, "webhook rejected")

			render.JSON(w, render.Err{Error: "unauthorized"}, http.StatusUnauthorized)
		case errors.Is(err, domain.ErrValidation):
			_ = render.Error(r.Context(), err, "webhook rejected")

			render.JSON(w, render.Err{Error: "invalid request"}, http.StatusBadRequest)
		default:
			render.InternalErrorJSON(r.Context(), w, err)
		}
	})
}

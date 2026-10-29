package ws

import (
	"net/http"

	"github.com/coder/websocket"
	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/pkg/render"
)

type Handler struct {
	hub *Hub
}

func NewHandler(hub *Hub) *Handler {
	return &Handler{
		hub: hub,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p, ok := principal.From(r.Context())
	if !ok {
		render.JSON(w, render.Err{Error: "unauthorized"}, http.StatusUnauthorized)

		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Debug().Err(err).Msg("ws: accept")

		return
	}

	ctx := conn.CloseRead(r.Context())

	unregister := h.hub.Register(ctx, p.PartyID, conn)
	defer unregister()

	<-ctx.Done()
}

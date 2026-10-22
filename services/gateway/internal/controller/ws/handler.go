package ws

import (
	"net/http"

	"github.com/coder/websocket"
	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/pkg/render"
)

// Handler upgrades an authenticated request to a WebSocket that receives the events of the caller's party.
type Handler struct {
	hub *Hub
}

func NewHandler(hub *Hub) *Handler {
	return &Handler{
		hub: hub,
	}
}

// ServeHTTP expects the router's authentication step to have put the caller into the request context.
// It blocks until the socket is gone.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p, ok := principal.From(r.Context())
	if !ok {
		render.JSON(w, render.Err{Error: "unauthorized"}, http.StatusUnauthorized)

		return
	}

	// Default options: the origin must be the request's own host. The desk is deployed same-origin
	// (the SPA and /api share one host), so a page from another site cannot use the visitor's cookie here.
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Debug().Err(err).Msg("ws: accept") // Accept has already answered the client

		return
	}

	// Clients only listen. Reading keeps pongs and the close handshake flowing (the hub's pings need it)
	// and cancels ctx once the peer is gone or the hub closed the socket.
	ctx := conn.CloseRead(r.Context())

	unregister := h.hub.Register(ctx, p.PartyID, conn)
	defer unregister()

	<-ctx.Done()
}

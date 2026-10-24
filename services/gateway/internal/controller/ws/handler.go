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
	hub            *Hub
	originPatterns []string
}

// NewHandler serves sockets whose Origin is the request's own host or matches one of originPatterns.
func NewHandler(hub *Hub, originPatterns ...string) *Handler {
	return &Handler{
		hub:            hub,
		originPatterns: originPatterns,
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

	// The origin must be the request's own host or one of the configured ones (the pages that may call the API
	// from elsewhere), so a page from any other site cannot ride the visitor's cookie.
	// Of the subprotocols a client offers only Protocol is ever accepted and echoed, which a browser insists on
	// when it offered any; the entry that carries its session token is never sent back.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols:   []string{Protocol},
		OriginPatterns: h.originPatterns,
	})
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

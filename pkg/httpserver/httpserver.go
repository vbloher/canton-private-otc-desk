package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

type Config struct {
	Host string `env:"HTTP_HOST"`
	Port string `env:"HTTP_PORT"`
}

type Server struct {
	server *http.Server
}

func New(ctx context.Context, handler http.Handler, c Config) (*Server, error) {
	httpServer := &http.Server{
		Handler:      handler,
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 20 * time.Second,
		Addr:         net.JoinHostPort(c.Host, c.Port),
	}

	var lc net.ListenConfig

	listener, err := lc.Listen(ctx, "tcp", httpServer.Addr)
	if err != nil {
		return nil, fmt.Errorf("net.Listen: %w", err)
	}

	s := &Server{
		server: httpServer,
	}

	go s.serve(listener)

	log.Info().Msg("http server: started on port: " + c.Port)

	return s, nil
}

func (s *Server) serve(listener net.Listener) {
	err := s.server.Serve(listener)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error().Err(err).Msg("http server: Serve")
	}
}

func (s *Server) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	err := s.server.Shutdown(ctx)
	if err != nil {
		log.Error().Err(err).Msg("http server: s.server.Shutdown")
	}

	log.Info().Msg("http server: closed")
}

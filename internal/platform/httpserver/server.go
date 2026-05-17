package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
)

type Server struct {
	engine *gin.Engine
	http   *http.Server
}

func New(cfg config.ServerConfig) *Server {
	if cfg.Mode != "" {
		gin.SetMode(cfg.Mode)
	}

	engine := gin.New()
	if len(cfg.TrustedProxies) == 0 {
		_ = engine.SetTrustedProxies(nil)
	} else {
		_ = engine.SetTrustedProxies(cfg.TrustedProxies)
	}

	readHeader := durOr(cfg.ReadHeaderTimeout, 5*time.Second)
	read := durOr(cfg.ReadTimeout, 30*time.Second)
	write := durOr(cfg.WriteTimeout, 30*time.Second)
	idle := durOr(cfg.IdleTimeout, 120*time.Second)
	maxHeader := cfg.MaxHeaderBytes
	if maxHeader <= 0 {
		maxHeader = 1 << 20
	}

	return &Server{
		engine: engine,
		http: &http.Server{
			Addr:              fmt.Sprintf(":%d", cfg.Port),
			Handler:           engine,
			ReadHeaderTimeout: readHeader,
			ReadTimeout:       read,
			WriteTimeout:      write,
			IdleTimeout:       idle,
			MaxHeaderBytes:    maxHeader,
		},
	}
}

func durOr(d, fallback time.Duration) time.Duration {
	if d <= 0 {
		return fallback
	}
	return d
}

func (s *Server) Engine() *gin.Engine { return s.engine }

func (s *Server) Start() error {
	if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

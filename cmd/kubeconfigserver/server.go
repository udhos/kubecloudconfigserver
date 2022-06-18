package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type server struct {
	server *http.Server
	router *gin.Engine
}

func newServer(addr string) *server {
	r := gin.New()
	return &server{
		router: r,
		server: &http.Server{Addr: addr, Handler: r},
	}
}

func (s *server) shutdown(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

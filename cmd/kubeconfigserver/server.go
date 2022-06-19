package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

/*
type server interface {
	getServer() *http.Server
	shutdown(timeout time.Duration)
}
*/

type serverHttp struct {
	server *http.Server
}

func newServerHttp(addr string, handler http.Handler) *serverHttp {
	return &serverHttp{
		server: &http.Server{Addr: addr, Handler: handler},
	}
}

/*
func (s *serverHttp) getServer() *http.Server {
	return s.server
}
*/

func (s *serverHttp) shutdown(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

type serverGin struct {
	server *http.Server
	router *gin.Engine
}

func newServerGin(addr string) *serverGin {
	r := gin.New()
	return &serverGin{
		router: r,
		server: &http.Server{Addr: addr, Handler: r},
	}
}

/*
func (s *serverGin) getServer() *http.Server {
	return s.server
}
*/

func (s *serverGin) shutdown(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

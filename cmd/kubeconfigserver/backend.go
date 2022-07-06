package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type backendError struct {
	status int
	err    error
}

func sendBackendError(c *gin.Context, err backendError) {
	switch err.status {
	case http.StatusNotFound:
		c.String(http.StatusNotFound, "not found")
		return
	}
	c.String(http.StatusBadGateway, "error status from backend: %d", err.status)
}

func httpSuccess(status int) bool {
	return status == 0 || (status >= 200 && status < 300)
}

func noError(status int, err error) bool {
	return err == nil && httpSuccess(status)
}

func newBackendError(status int, err error) error {
	if noError(status, err) {
		return nil
	}
	return backendError{
		status: status,
		err:    err,
	}
}

func (e backendError) Error() string {
	if noError(e.status, e.err) {
		return "<nil>"
	}
	var msg string
	if e.err == nil {
		msg = "<nil>"
	} else {
		msg = e.err.Error()
	}
	return fmt.Sprintf("backendError: status:%d error:%s", e.status, msg)
}

type backend interface {
	fetch(ctx context.Context, path string) ([]byte, error)
}

func newBackend(tracer trace.Tracer, address, options string) backend {
	if dir := strings.TrimPrefix(address, "dir:"); dir != address {
		log.Printf("backend: %s: dir", address)
		return newBackendDir(tracer, dir, options)
	}
	log.Printf("backend: %s: http", address)
	return &backendHTTP{tracer: tracer, host: address}
}

type backendDir struct {
	tracer  trace.Tracer
	dir     string
	flatten bool // strip directory prefixes from requested path
}

func newBackendDir(tracer trace.Tracer, dir, options string) *backendDir {
	flatten := strings.Contains(options, "flatten")
	return &backendDir{tracer: tracer, dir: dir, flatten: flatten}
}

func (b *backendDir) fetch(ctx context.Context, path string) ([]byte, error) {
	_, span := b.tracer.Start(ctx, "backendDir.fetch")
	defer span.End()

	var status int
	var filename string
	if b.flatten {
		filename = filepath.Base(path)
	} else {
		filename = path
	}
	fullpath := filepath.Join(b.dir, filename)
	data, err := os.ReadFile(fullpath)
	if err != nil && strings.Contains(err.Error(), "no such file or directory") {
		status = http.StatusNotFound
	}
	log.Printf("backendDir: flatten=%t path='%s' filename='%s' fullpath='%s' size=%d status=%d error:%v",
		b.flatten, path, filename, fullpath, len(data), status, err)

	be := newBackendError(status, err)
	if be != nil {
		span.SetStatus(codes.Error, be.Error())
	}

	return data, be
}

type backendHTTP struct {
	tracer trace.Tracer
	host   string
}

func (b *backendHTTP) fetch(ctx context.Context, path string) ([]byte, error) {
	newCtx, span := b.tracer.Start(ctx, "backendHTTP.fetch")
	defer span.End()

	var status int
	u, errJoin := url.JoinPath(b.host, path)
	if errJoin != nil {
		log.Printf("backendHTTP: path='%s' join error: %v", path, errJoin)
		be := newBackendError(status, errJoin)
		span.SetStatus(codes.Error, be.Error())
		return nil, be
	}

	req, errReq := http.NewRequestWithContext(newCtx, "GET", u, nil)
	if errReq != nil {
		log.Printf("backendHTTP: path='%s' req error: %v", path, errReq)
		be := newBackendError(status, errReq)
		span.SetStatus(codes.Error, be.Error())
		return nil, be
	}

	client := http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

	resp, errGet := client.Do(req)
	if errGet != nil {
		log.Printf("backendHTTP: path='%s' url='%s' GET error: %v",
			path, u, errGet)
		be := newBackendError(status, errGet)
		span.SetStatus(codes.Error, be.Error())
		return nil, be
	}
	defer resp.Body.Close()
	status = resp.StatusCode
	data, errRead := io.ReadAll(resp.Body)
	resp.Body.Close()
	log.Printf("backendHTTP: path='%s' url='%s' size=%d status=%d error:%v",
		path, u, len(data), status, errRead)
	be := newBackendError(status, errRead)
	if be != nil {
		span.SetStatus(codes.Error, be.Error())
	}
	return data, be
}

package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
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
	fetch(path string) ([]byte, error)
}

func newBackend(address, options string) backend {
	if dir := strings.TrimPrefix(address, "dir:"); dir != address {
		log.Printf("backend: %s: dir", address)
		return newBackendDir(dir, options)
	}
	log.Printf("backend: %s: http", address)
	return &backendHTTP{address}
}

type backendDir struct {
	dir     string
	flatten bool // strip directory prefixes from requested path
}

func newBackendDir(dir, options string) *backendDir {
	flatten := strings.Contains(options, "flatten")
	return &backendDir{dir: dir, flatten: flatten}
}

func (b *backendDir) fetch(path string) ([]byte, error) {
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
	return data, newBackendError(status, err)
}

type backendHTTP struct {
	host string
}

func (b *backendHTTP) fetch(path string) ([]byte, error) {
	var status int
	url := b.host + path
	resp, errGet := http.Get(url)
	if errGet != nil {
		log.Printf("backendHTTP: path='%s' url='%s' error: %v",
			path, url, errGet)
		return nil, newBackendError(status, errGet)
	}
	defer resp.Body.Close()
	status = resp.StatusCode
	data, errRead := io.ReadAll(resp.Body)
	log.Printf("backendHTTP: path='%s' url='%s' size=%d status=%d error:%v",
		path, url, len(data), status, errRead)
	return data, newBackendError(status, errRead)
}

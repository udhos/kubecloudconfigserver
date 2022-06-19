package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

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
	var filename string
	if b.flatten {
		filename = filepath.Base(path)
	} else {
		filename = path
	}
	fullpath := filepath.Join(b.dir, filename)
	data, err := os.ReadFile(fullpath)
	log.Printf("backendDir: flatten=%t path='%s' filename='%s' fullpath='%s' size=%d error:%v",
		b.flatten, path, filename, fullpath, len(data), err)
	return data, err
}

type backendHTTP struct {
	host string
}

func (b *backendHTTP) fetch(path string) ([]byte, error) {
	url := b.host + path
	resp, errGet := http.Get(url)
	if errGet != nil {
		log.Printf("backendHTTP: path='%s' url='%s' error: %v",
			path, url, errGet)
		return nil, errGet
	}
	defer resp.Body.Close()
	data, errRead := io.ReadAll(resp.Body)
	log.Printf("backendHTTP: path='%s' url='%s' size=%d error:%v",
		path, url, len(data), errRead)
	return data, errRead
}

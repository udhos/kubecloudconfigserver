package main

import (
	"path/filepath"
	"strings"
	"sync"
)

type table struct {
	tab   map[string]struct{}
	mutex sync.Mutex
}

func newTable() *table {
	return &table{
		tab: map[string]struct{}{},
	}
}

func (t *table) add(key string) {
	t.mutex.Lock()
	t.tab[key] = struct{}{}
	t.mutex.Unlock()
}

func (t *table) del(key string) {
	t.mutex.Lock()
	delete(t.tab, key)
	t.mutex.Unlock()
}

func (t *table) match(app string) []string {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	var keys []string
	for k := range t.tab {
		// k: /path/to/config1-default.yml,config2-default.yml,config3-default.yml
		base := filepath.Base(k)
		// base: config1-default.yml,config2-default.yml,config3-default.yml
		// app: config2
		if strings.Contains(base, app) {
			keys = append(keys, k)
		}
	}
	return keys
}

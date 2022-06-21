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
		if refreshMatch(app, k) {
			keys = append(keys, k)
		}
	}
	return keys
}

func refreshMatch(app, key string) bool {
	app = strings.TrimSuffix(app, ":**")    // "config:file2:**" -> "config:file2"
	app = strings.Replace(app, ":", "-", 1) // "config:file2" -> "config-file2"

	// "/path/to/config-file1-default.yml,config-file2-default.yml,config-file3-default.yml" ->
	// "config-file1-default.yml,config-file2-default.yml,config-file3-default.yml"
	base := filepath.Base(key)

	// "config-file1-default.yml,config-file2-default.yml,config-file3-default.yml" ->
	// "config-file1-default.yml"
	keyFiles := strings.Split(base, ",")
	for _, kf := range keyFiles {
		if strings.HasPrefix(kf, app) {
			return true
		}
	}
	return false
}

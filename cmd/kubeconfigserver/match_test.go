package main

import (
	"testing"
)

type testMatch struct {
	application    string
	key            string
	expectedResult bool
}

var testMatchTable = []testMatch{
	{"config-file2:**", "", false},
	{"config:file2:**", "", false},
	{"config-file2:**", "config-file2-default.yml", true},
	{"config:file2:**", "config-file2-default.yml", true},
	{"config-file4:**", "config-file2-default.yml", false},
	{"config:file4:**", "config-file2-default.yml", false},
	{"config-file2:**", "/path/to/config-file2-default.yml", true},
	{"config:file2:**", "/path/to/config-file2-default.yml", true},
	{"config-file4:**", "/path/to/config-file2-default.yml", false},
	{"config:file4:**", "/path/to/config-file2-default.yml", false},
	{"config-file2:**", "/path/to/config-file1-default.yml,config-file2-default.yml,config-file3-default.yml", true},
	{"config:file2:**", "/path/to/config-file1-default.yml,config-file2-default.yml,config-file3-default.yml", true},
	{"config-file4:**", "/path/to/config-file1-default.yml,config-file2-default.yml,config-file3-default.yml", false},
	{"config:file4:**", "/path/to/config-file1-default.yml,config-file2-default.yml,config-file3-default.yml", false},
}

func TestMatch(t *testing.T) {
	for _, data := range testMatchTable {
		result := refreshMatch(data.application, data.key)
		if result != data.expectedResult {
			t.Errorf("application='%s' key='%s' expected=%t got=%t",
				data.application, data.key, data.expectedResult, result)
		}
	}
}

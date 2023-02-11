package main

import (
	"strings"
	"testing"
)

func TestGetIp(t *testing.T) {
	ip := getIp("AAAA")
	if !strings.Contains(ip, ":") {
		t.Fatal(`TestGetIp AAAA error`)
	}

	ip = getIp("A")
	if !strings.Contains(ip, ".") {
		t.Fatal(`TestGetIp A error`)
	}
}

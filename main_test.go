package main

import (
	"fmt"
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

func TestGetPrefix(t *testing.T) {
	prefix := getPrefix(4)
	fmt.Printf("prefix| %s\n", prefix)
}

package main

import (
	"fmt"
	"testing"
)

func TestGetIp(t *testing.T) {
	ipv4, ipv6 := fetchIp()
	fmt.Printf("GetIp: %s, %s\n", ipv4, ipv6)
}

func TestGetPrefix(t *testing.T) {
	prefix := getPrefix(4, "240e:3b2:7e5c:4360:ff18:361d:a5c8:f22c")
	fmt.Printf("prefix| %s\n", prefix)
}

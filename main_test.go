package main

import (
	"fmt"
	"net"
	"testing"
)

func TestGetIp(t *testing.T) {
	ipv4, ipv6 := fetchIp()
	fmt.Printf("GetIp: %s, %s\n", ipv4, ipv6)
}

func TestBuildNewIpv6(t *testing.T) {
	r := dnsRecord{
		IP:     "240e::4360:ff18:361d:a5c8:f22c",
		Prefix: 64,
	}
	newIp := net.ParseIP("240e::abcd:ff18:361d:a5c8:f22c")
	ip := buildNewIpv6(&r, &newIp)
	if ip != "240e::abcd:ff18:361d:a5c8:f22c" {
		t.FailNow()
	}
}

func TestFetchLocalIp(t *testing.T) {
	mac := "0A:00:27:00:00:0A"
	ipv4, ipv6 := findLocalIp(mac)
	fmt.Printf("Found: %s|%s\n", ipv4, ipv6)
}

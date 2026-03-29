package telegram

import (
	"context"
	"net"
	"testing"
)

func TestRedisIPAllocator_StartsFromDot3WithoutRedis(t *testing.T) {
	alloc := RedisIPAllocator{SubnetCIDR: "10.8.1.0/24"}
	ip, err := alloc.Allocate(context.Background(), "n1")
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if ip != "10.8.1.3/32" {
		t.Fatalf("unexpected ip %s", ip)
	}
}

func TestHostIPFromSubnet(t *testing.T) {
	_, n, err := net.ParseCIDR("10.8.1.0/24")
	if err != nil {
		t.Fatalf("parse cidr: %v", err)
	}
	if got := hostIP(n, 3); got != "10.8.1.3/32" {
		t.Fatalf("unexpected host ip %s", got)
	}
}

func TestHostCapacity(t *testing.T) {
	_, n, err := net.ParseCIDR("10.8.1.0/24")
	if err != nil {
		t.Fatalf("parse cidr: %v", err)
	}
	if cap := hostCapacity(n); cap != 252 {
		t.Fatalf("unexpected capacity %d", cap)
	}
}

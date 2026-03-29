package telegram

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisIPAllocator struct {
	Redis      *redis.Client
	SubnetCIDR string
}

func (a RedisIPAllocator) Allocate(ctx context.Context, nodeID string) (string, error) {
	netw, err := a.subnet()
	if err != nil {
		return "", err
	}
	if a.Redis == nil {
		return hostIP(netw, 3), nil
	}

	counterKey := "ipalloc:" + nodeID
	usedKey := "ipalloc:used:" + nodeID
	capacity := hostCapacity(netw)
	if capacity <= 0 {
		return "", fmt.Errorf("subnet has no host capacity: %s", netw.String())
	}

	for i := 0; i < capacity; i++ {
		counter, err := a.Redis.Incr(ctx, counterKey).Result()
		if err != nil {
			return "", err
		}
		if counter == 1 {
			_ = a.Redis.Expire(ctx, counterKey, 365*24*time.Hour).Err()
		}
		host := 3 + int((counter-1)%int64(capacity))
		candidate := hostIP(netw, host)
		if candidate == "" {
			continue
		}
		ok, err := a.Redis.SAdd(ctx, usedKey, candidate).Result()
		if err != nil {
			return "", err
		}
		if ok == 1 {
			_ = a.Redis.Expire(ctx, usedKey, 365*24*time.Hour).Err()
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no free ip available in subnet %s", netw.String())
}

func (a RedisIPAllocator) subnet() (*net.IPNet, error) {
	cidr := a.SubnetCIDR
	if cidr == "" {
		cidr = "10.8.1.0/24"
	}
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid subnet cidr %q: %w", cidr, err)
	}
	if n.IP.To4() == nil {
		return nil, fmt.Errorf("only IPv4 subnet is supported: %s", cidr)
	}
	return n, nil
}

func hostIP(subnet *net.IPNet, host int) string {
	base := subnet.IP.To4()
	if base == nil || host < 0 || host > 255 {
		return ""
	}
	ip := net.IPv4(base[0], base[1], base[2], byte(host)).String()
	if !subnet.Contains(net.ParseIP(ip)) {
		return ""
	}
	return ip + "/32"
}

func hostCapacity(subnet *net.IPNet) int {
	ones, bits := subnet.Mask.Size()
	if bits != 32 {
		return 0
	}
	hostBits := bits - ones
	if hostBits <= 0 {
		return 0
	}
	total := 1 << hostBits
	reservedPrefix := 3                     // .0 network, .1/.2 reserved-existing
	available := total - reservedPrefix - 1 // broadcast
	if available < 1 {
		return 0
	}
	return available
}

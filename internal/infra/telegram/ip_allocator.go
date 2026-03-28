package telegram

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisIPAllocator struct {
	Redis *redis.Client
}

func (a RedisIPAllocator) Allocate(ctx context.Context, nodeID string) (string, error) {
	if a.Redis == nil {
		return "10.200.0.2/32", nil
	}
	counter, err := a.Redis.Incr(ctx, "ipalloc:"+nodeID).Result()
	if err != nil {
		return "", err
	}
	if counter == 1 {
		_ = a.Redis.Expire(ctx, "ipalloc:"+nodeID, 365*24*time.Hour).Err()
	}
	host := int(counter%250) + 2
	third := int((counter / 250) % 250)
	return fmt.Sprintf("10.200.%d.%d/32", third, host), nil
}

package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type DialogueState string

const (
	StateIdle                    DialogueState = "idle"
	StateAwaitingInviteCode      DialogueState = "awaiting_invite_code"
	StateAwaitingAdminUserLookup DialogueState = "awaiting_admin_user_lookup"
	StateAwaitingBalanceAdjust   DialogueState = "awaiting_balance_adjustment"
	StateAwaitingConfirmReissue  DialogueState = "awaiting_confirm_reissue"
	StateAwaitingConfirmBlock    DialogueState = "awaiting_confirm_block"
	StateAwaitingConfirmUnblock  DialogueState = "awaiting_confirm_unblock"

	StateAwaitBatchCnt DialogueState = "await_batch_count"
	StateAwaitUserStat DialogueState = "await_user_stat"
	StateAwaitRevokeID DialogueState = "await_revoke_id"
)

type UserState struct {
	State     DialogueState `json:"state"`
	UpdatedAt time.Time     `json:"updated_at"`
}

type StateStore interface {
	Get(ctx context.Context, telegramID int64) (UserState, error)
	Set(ctx context.Context, telegramID int64, state DialogueState) error
	Clear(ctx context.Context, telegramID int64) error
}

type RedisStateStore struct {
	Redis *redis.Client
	TTL   time.Duration
}

func (s RedisStateStore) key(id int64) string { return fmt.Sprintf("tg:state:%d", id) }

func (s RedisStateStore) Get(ctx context.Context, telegramID int64) (UserState, error) {
	if s.Redis == nil {
		return UserState{State: StateIdle}, nil
	}

	raw, err := s.Redis.Get(ctx, s.key(telegramID)).Result()
	if err == redis.Nil {
		return UserState{State: StateIdle}, nil
	}
	if err != nil {
		return UserState{}, err
	}

	var out UserState
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return UserState{}, err
	}
	if out.State == "" {
		out.State = StateIdle
	}
	return out, nil
}

func (s RedisStateStore) Set(ctx context.Context, telegramID int64, state DialogueState) error {
	if s.Redis == nil {
		return nil
	}
	if s.TTL <= 0 {
		s.TTL = 24 * time.Hour
	}
	payload, _ := json.Marshal(UserState{State: state, UpdatedAt: time.Now().UTC()})
	return s.Redis.Set(ctx, s.key(telegramID), payload, s.TTL).Err()
}

func (s RedisStateStore) Clear(ctx context.Context, telegramID int64) error {
	if s.Redis == nil {
		return nil
	}
	return s.Redis.Del(ctx, s.key(telegramID)).Err()
}

type MemoryStateStore struct {
	mu sync.RWMutex
	m  map[int64]UserState
}

func (s *MemoryStateStore) Get(_ context.Context, telegramID int64) (UserState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.m == nil {
		return UserState{State: StateIdle}, nil
	}
	st, ok := s.m[telegramID]
	if !ok {
		return UserState{State: StateIdle}, nil
	}
	return st, nil
}

func (s *MemoryStateStore) Set(_ context.Context, telegramID int64, state DialogueState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[int64]UserState{}
	}
	s.m[telegramID] = UserState{State: state, UpdatedAt: time.Now().UTC()}
	return nil
}

func (s *MemoryStateStore) Clear(_ context.Context, telegramID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m != nil {
		delete(s.m, telegramID)
	}
	return nil
}

package runtime

import (
	"context"
	"errors"
	"fmt"
)

type unavailableRuntime struct {
	cause error
}

func NewUnavailableRuntime(cause error) VPNRuntime {
	if cause == nil {
		cause = errors.New("unknown runtime init error")
	}
	return unavailableRuntime{cause: cause}
}

func (u unavailableRuntime) ApplyPeer(context.Context, PeerOperationRequest) (OperationResult, error) {
	return OperationResult{}, fmt.Errorf("%w: %v", ErrUnavailable, u.cause)
}

func (u unavailableRuntime) RevokePeer(context.Context, PeerOperationRequest) (OperationResult, error) {
	return OperationResult{}, fmt.Errorf("%w: %v", ErrUnavailable, u.cause)
}

func (u unavailableRuntime) ListPeerStats(context.Context) ([]PeerStat, error) {
	return nil, fmt.Errorf("%w: %v", ErrUnavailable, u.cause)
}

func (u unavailableRuntime) Health(context.Context) error {
	return fmt.Errorf("%w: %v", ErrUnavailable, u.cause)
}

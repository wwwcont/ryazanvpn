package peer

import "time"

// Peer describes desired runtime peer state authored by control-plane.
// Runtime adapters must only apply/revoke peers that exist in this model.
type Peer struct {
	DeviceAccessID string
	DeviceID       string
	VPNNodeID      string
	Protocol       string
	PublicKey      string
	AssignedIP     string
	Keepalive      int
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

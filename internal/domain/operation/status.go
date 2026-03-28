package operation

const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

const (
	TypeCreatePeer   = "create_peer"
	TypeRevokePeer   = "revoke_peer"
	TypeRotateKeys   = "rotate_keys"
	TypeReloadConfig = "reload_config"
	TypeRestart      = "restart"
)

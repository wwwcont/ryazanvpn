package nodeapi

const (
	HeaderProtocolVersion   = "X-Protocol-Version"
	CurrentProtocolVersion  = "1"
	PreviousProtocolVersion = "0"
)

func IsSupportedProtocolVersion(version string) bool {
	return version == CurrentProtocolVersion || version == PreviousProtocolVersion
}

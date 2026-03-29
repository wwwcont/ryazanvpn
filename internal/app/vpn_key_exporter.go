package app

import "context"

// DefaultVPNAWGFields contains AmneziaWG obfuscation knobs required by DefaultVPN format.
type DefaultVPNAWGFields struct {
	Jc   int
	Jmin int
	Jmax int
	S1   int
	S2   int
	S3   int
	S4   int
	H1   string
	H2   string
	H3   string
	H4   string
	I1   string
	I2   string
	I3   string
	I4   string
	I5   string
	MTU  int
}

type ExportVPNKeyInput struct {
	Config             string
	Description        string
	HostName           string
	Port               int
	DNS1               string
	DNS2               string
	ProtocolVersion    int
	TransportProto     string
	SubnetAddress      string
	ClientPublicKey    string
	DefaultContainerID string
	AWG                DefaultVPNAWGFields
}

type VPNKeyExporter interface {
	ExportDefaultVPN(ctx context.Context, in ExportVPNKeyInput) (string, error)
}

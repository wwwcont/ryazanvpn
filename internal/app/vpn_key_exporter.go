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
	H1   int
	H2   int
	H3   int
	H4   int
	I1   int
	I2   int
	I3   int
	I4   int
	I5   int
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

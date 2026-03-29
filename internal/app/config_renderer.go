package app

type RenderAmneziaWGInput struct {
	DevicePrivateKey string
	ServerPublicKey  string
	PresharedKey     string
	AssignedIP       string
	MTU              int
	DNS              []string
	EndpointHost     string
	EndpointPort     int
	Keepalive        int
	AllowedIPs       []string
	AWG              DefaultVPNAWGFields
}

type ConfigRenderer interface {
	RenderAmneziaWG(in RenderAmneziaWGInput) (string, error)
}

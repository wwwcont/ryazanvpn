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

type RenderXrayRealityInput struct {
	DeviceID     string
	DevicePublic string
	ServerName   string
	ServerHost   string
	ServerPort   int
	UserUUID     string
}

type ConfigRenderer interface {
	RenderAmneziaWG(in RenderAmneziaWGInput) (string, error)
	RenderXrayReality(in RenderXrayRealityInput) (string, error)
}

package app

type RenderAmneziaWGInput struct {
	DevicePrivateKey string
	ServerPublicKey  string
	PresharedKey     string
	AssignedIP       string
	DNS              []string
	EndpointHost     string
	EndpointPort     int
	Keepalive        int
	AllowedIPs       []string
}

type ConfigRenderer interface {
	RenderAmneziaWG(in RenderAmneziaWGInput) (string, error)
}

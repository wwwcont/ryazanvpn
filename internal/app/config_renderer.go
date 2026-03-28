package app

type RenderAmneziaWGInput struct {
	DevicePrivateKey string
	ServerPublicKey  string
	AssignedIP       string
	DNS              []string
	EndpointHost     string
	EndpointPort     int
	Keepalive        int
}

type ConfigRenderer interface {
	RenderAmneziaWG(in RenderAmneziaWGInput) (string, error)
}

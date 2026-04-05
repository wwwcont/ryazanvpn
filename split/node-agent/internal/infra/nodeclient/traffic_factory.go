package nodeclient

import (
	"strings"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/app"
)

type TrafficFactory struct {
	Secret     string
	Timeout    time.Duration
	MaxRetries int
}

func (f TrafficFactory) ForNode(agentBaseURL string) app.NodeTrafficClient {
	base := strings.TrimSpace(agentBaseURL)
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "http://" + base
	}
	cli := New(Config{BaseURL: base, Secret: f.Secret, Timeout: f.Timeout, MaxRetries: f.MaxRetries})
	return AppAdapter{Client: cli}
}

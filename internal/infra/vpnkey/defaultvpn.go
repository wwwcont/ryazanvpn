package vpnkey

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/wwwcont/ryazanvpn/internal/app"
)

const defaultVPNPrefix = "vpn://"

type DefaultVPNExporter struct{}

func NewDefaultVPNExporter() *DefaultVPNExporter {
	return &DefaultVPNExporter{}
}

type defaultVPNEnvelope struct {
	Containers       []defaultVPNContainer `json:"containers"`
	DefaultContainer string                `json:"defaultContainer"`
	Description      string                `json:"description"`
	DNS1             string                `json:"dns1"`
	DNS2             string                `json:"dns2"`
	HostName         string                `json:"hostName"`
}

type defaultVPNContainer struct {
	AWG defaultVPNAWG `json:"awg"`
}

type defaultVPNAWG struct {
	Jc              int    `json:"Jc"`
	Jmin            int    `json:"Jmin"`
	Jmax            int    `json:"Jmax"`
	S1              int    `json:"S1"`
	S2              int    `json:"S2"`
	S3              int    `json:"S3"`
	S4              int    `json:"S4"`
	H1              int    `json:"H1"`
	H2              int    `json:"H2"`
	H3              int    `json:"H3"`
	H4              int    `json:"H4"`
	I1              int    `json:"I1"`
	I2              int    `json:"I2"`
	I3              int    `json:"I3"`
	I4              int    `json:"I4"`
	I5              int    `json:"I5"`
	Port            int    `json:"port"`
	ProtocolVersion int    `json:"protocol_version"`
	SubnetAddress   string `json:"subnet_address"`
	TransportProto  string `json:"transport_proto"`
	LastConfig      string `json:"last_config"`
}

type defaultVPNLastConfig struct {
	ClientIP            string `json:"client_ip"`
	ClientPrivateKey    string `json:"client_priv_key"`
	ClientPublicKey     string `json:"client_pub_key"`
	PSKKey              string `json:"psk_key"`
	ServerPublicKey     string `json:"server_pub_key"`
	HostName            string `json:"hostName"`
	Port                int    `json:"port"`
	PersistentKeepAlive int    `json:"persistent_keep_alive"`
	AllowedIPs          string `json:"allowed_ips"`
	Config              string `json:"config"`
}

type wgConfigParsed struct {
	ClientPrivateKey    string
	ClientIP            string
	ServerPublicKey     string
	PresharedKey        string
	EndpointHost        string
	EndpointPort        int
	AllowedIPs          string
	PersistentKeepalive int
}

func (e *DefaultVPNExporter) ExportDefaultVPN(_ context.Context, in app.ExportVPNKeyInput) (string, error) {
	parsed, err := ParseWGConfig(in.Config)
	if err != nil {
		return "", err
	}

	host := strings.TrimSpace(in.HostName)
	if host == "" {
		host = parsed.EndpointHost
	}
	port := in.Port
	if port <= 0 {
		port = parsed.EndpointPort
	}

	lastCfg := defaultVPNLastConfig{
		ClientIP:            parsed.ClientIP,
		ClientPrivateKey:    parsed.ClientPrivateKey,
		ClientPublicKey:     strings.TrimSpace(in.ClientPublicKey),
		PSKKey:              parsed.PresharedKey,
		ServerPublicKey:     parsed.ServerPublicKey,
		HostName:            host,
		Port:                port,
		PersistentKeepAlive: parsed.PersistentKeepalive,
		AllowedIPs:          parsed.AllowedIPs,
		Config:              in.Config,
	}
	if lastCfg.ClientPublicKey == "" {
		return "", fmt.Errorf("missing client public key")
	}
	lastCfgJSON, err := json.Marshal(lastCfg)
	if err != nil {
		return "", err
	}

	containerID := strings.TrimSpace(in.DefaultContainerID)
	if containerID == "" {
		containerID = "awg"
	}

	envelope := defaultVPNEnvelope{
		Containers: []defaultVPNContainer{{
			AWG: defaultVPNAWG{
				Jc:              in.AWG.Jc,
				Jmin:            in.AWG.Jmin,
				Jmax:            in.AWG.Jmax,
				S1:              in.AWG.S1,
				S2:              in.AWG.S2,
				S3:              in.AWG.S3,
				S4:              in.AWG.S4,
				H1:              in.AWG.H1,
				H2:              in.AWG.H2,
				H3:              in.AWG.H3,
				H4:              in.AWG.H4,
				I1:              in.AWG.I1,
				I2:              in.AWG.I2,
				I3:              in.AWG.I3,
				I4:              in.AWG.I4,
				I5:              in.AWG.I5,
				Port:            port,
				ProtocolVersion: defaultInt(in.ProtocolVersion, 1),
				SubnetAddress:   firstNonEmpty(strings.TrimSpace(in.SubnetAddress), parsed.ClientIP),
				TransportProto:  firstNonEmpty(strings.TrimSpace(in.TransportProto), "udp"),
				LastConfig:      string(lastCfgJSON),
			},
		}},
		DefaultContainer: containerID,
		Description:      firstNonEmpty(strings.TrimSpace(in.Description), "RyazanVPN"),
		DNS1:             firstNonEmpty(strings.TrimSpace(in.DNS1), "1.1.1.1"),
		DNS2:             firstNonEmpty(strings.TrimSpace(in.DNS2), "8.8.8.8"),
		HostName:         host,
	}

	return EncodeDefaultVPN(envelope)
}

func EncodeDefaultVPN(envelope defaultVPNEnvelope) (string, error) {
	jsonBlob, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(jsonBlob); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	out := make([]byte, 4+compressed.Len())
	binary.BigEndian.PutUint32(out[:4], uint32(len(jsonBlob)))
	copy(out[4:], compressed.Bytes())
	return defaultVPNPrefix + base64.RawURLEncoding.EncodeToString(out), nil
}

func DecodeDefaultVPN(key string) (defaultVPNEnvelope, error) {
	if !strings.HasPrefix(key, defaultVPNPrefix) {
		return defaultVPNEnvelope{}, fmt.Errorf("invalid vpn key prefix")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(key, defaultVPNPrefix))
	if err != nil {
		return defaultVPNEnvelope{}, err
	}
	if len(raw) < 5 {
		return defaultVPNEnvelope{}, fmt.Errorf("vpn key payload too short")
	}
	expectedLen := binary.BigEndian.Uint32(raw[:4])
	zr, err := zlib.NewReader(bytes.NewReader(raw[4:]))
	if err != nil {
		return defaultVPNEnvelope{}, err
	}
	defer zr.Close()
	var jsonBlob bytes.Buffer
	if _, err := jsonBlob.ReadFrom(zr); err != nil {
		return defaultVPNEnvelope{}, err
	}
	if expectedLen != uint32(jsonBlob.Len()) {
		return defaultVPNEnvelope{}, fmt.Errorf("json length mismatch: expected=%d got=%d", expectedLen, jsonBlob.Len())
	}
	var envelope defaultVPNEnvelope
	if err := json.Unmarshal(jsonBlob.Bytes(), &envelope); err != nil {
		return defaultVPNEnvelope{}, err
	}
	return envelope, nil
}

func ParseWGConfig(cfg string) (wgConfigParsed, error) {
	var out wgConfigParsed
	lines := strings.Split(cfg, "\n")
	section := ""
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")))
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(parts[0]))
		v := strings.TrimSpace(parts[1])
		switch section {
		case "interface":
			switch k {
			case "privatekey":
				out.ClientPrivateKey = v
			case "address":
				if out.ClientIP == "" {
					out.ClientIP = strings.TrimSpace(strings.Split(v, ",")[0])
				}
			}
		case "peer":
			switch k {
			case "publickey":
				out.ServerPublicKey = v
			case "presharedkey":
				out.PresharedKey = v
			case "endpoint":
				host, port, err := splitHostPort(v)
				if err != nil {
					return out, fmt.Errorf("parse endpoint: %w", err)
				}
				out.EndpointHost = host
				out.EndpointPort = port
			case "allowedips":
				out.AllowedIPs = v
			case "persistentkeepalive":
				fmt.Sscanf(v, "%d", &out.PersistentKeepalive)
			}
		}
	}
	if out.ClientPrivateKey == "" || out.ClientIP == "" || out.ServerPublicKey == "" || out.PresharedKey == "" || out.EndpointHost == "" || out.EndpointPort <= 0 {
		return out, fmt.Errorf("wireguard config is incomplete")
	}
	if out.PersistentKeepalive <= 0 {
		out.PersistentKeepalive = 25
	}
	if out.AllowedIPs == "" {
		out.AllowedIPs = "0.0.0.0/0, ::/0"
	}
	return out, nil
}

func splitHostPort(endpoint string) (string, int, error) {
	host, portRaw, err := net.SplitHostPort(endpoint)
	if err != nil {
		if strings.Count(endpoint, ":") == 1 {
			parts := strings.SplitN(endpoint, ":", 2)
			host = parts[0]
			portRaw = parts[1]
		} else {
			return "", 0, err
		}
	}
	var port int
	if _, err := fmt.Sscanf(portRaw, "%d", &port); err != nil {
		return "", 0, err
	}
	return host, port, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func defaultInt(v, fallback int) int {
	if v <= 0 {
		return fallback
	}
	return v
}

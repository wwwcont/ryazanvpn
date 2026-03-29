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
	"net/netip"
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
	Container string        `json:"container"`
	AWG       defaultVPNAWG `json:"awg"`
}

type defaultVPNAWG struct {
	Jc              int    `json:"Jc"`
	Jmin            int    `json:"Jmin"`
	Jmax            int    `json:"Jmax"`
	S1              int    `json:"S1"`
	S2              int    `json:"S2"`
	S3              int    `json:"S3"`
	S4              int    `json:"S4"`
	H1              string `json:"H1"`
	H2              string `json:"H2"`
	H3              string `json:"H3"`
	H4              string `json:"H4"`
	I1              string `json:"I1"`
	I2              string `json:"I2"`
	I3              string `json:"I3"`
	I4              string `json:"I4"`
	I5              string `json:"I5"`
	Port            int    `json:"port"`
	ProtocolVersion int    `json:"protocol_version"`
	SubnetAddress   string `json:"subnet_address"`
	TransportProto  string `json:"transport_proto"`
	LastConfig      string `json:"last_config"`
}

type defaultVPNLastConfig struct {
	ClientID            string   `json:"clientId"`
	MTU                 int      `json:"mtu"`
	Jc                  int      `json:"Jc"`
	Jmin                int      `json:"Jmin"`
	Jmax                int      `json:"Jmax"`
	S1                  int      `json:"S1"`
	S2                  int      `json:"S2"`
	S3                  int      `json:"S3"`
	S4                  int      `json:"S4"`
	H1                  string   `json:"H1"`
	H2                  string   `json:"H2"`
	H3                  string   `json:"H3"`
	H4                  string   `json:"H4"`
	I1                  string   `json:"I1"`
	I2                  string   `json:"I2"`
	I3                  string   `json:"I3"`
	I4                  string   `json:"I4"`
	I5                  string   `json:"I5"`
	ClientIP            string   `json:"client_ip"`
	ClientPrivateKey    string   `json:"client_priv_key"`
	ClientPublicKey     string   `json:"client_pub_key"`
	PSKKey              string   `json:"psk_key"`
	ServerPublicKey     string   `json:"server_pub_key"`
	HostName            string   `json:"hostName"`
	Port                int      `json:"port"`
	PersistentKeepAlive int      `json:"persistent_keep_alive"`
	AllowedIPs          []string `json:"allowed_ips"`
	Config              string   `json:"config"`
}

type wgConfigParsed struct {
	ClientID            string
	MTU                 int
	Jc                  int
	Jmin                int
	Jmax                int
	S1                  int
	S2                  int
	S3                  int
	S4                  int
	H1                  string
	H2                  string
	H3                  string
	H4                  string
	I1                  string
	I2                  string
	I3                  string
	I4                  string
	I5                  string
	ClientPrivateKey    string
	ClientIP            string
	ServerPublicKey     string
	PresharedKey        string
	EndpointHost        string
	EndpointPort        int
	AllowedIPs          []string
	PersistentKeepalive int
}

func (e *DefaultVPNExporter) ExportDefaultVPN(_ context.Context, in app.ExportVPNKeyInput) (string, error) {
	parsed, err := ParseWGConfig(in.Config)
	if err != nil {
		return "", err
	}

	host := firstNonEmpty(strings.TrimSpace(in.HostName), parsed.EndpointHost)
	port := defaultInt(in.Port, parsed.EndpointPort)
	if port <= 0 {
		return "", fmt.Errorf("invalid port")
	}
	clientPub := strings.TrimSpace(in.ClientPublicKey)
	if clientPub == "" {
		clientPub = strings.TrimSpace(parsed.ClientID)
	}
	if clientPub == "" {
		return "", fmt.Errorf("missing client public key")
	}

	awg := normalizeAWG(in.AWG, parsed)
	lastCfg := defaultVPNLastConfig{
		ClientID:            clientPub,
		MTU:                 defaultInt(awg.MTU, 1376),
		Jc:                  awg.Jc,
		Jmin:                awg.Jmin,
		Jmax:                awg.Jmax,
		S1:                  awg.S1,
		S2:                  awg.S2,
		S3:                  awg.S3,
		S4:                  awg.S4,
		H1:                  awg.H1,
		H2:                  awg.H2,
		H3:                  awg.H3,
		H4:                  awg.H4,
		I1:                  awg.I1,
		I2:                  awg.I2,
		I3:                  awg.I3,
		I4:                  awg.I4,
		I5:                  awg.I5,
		ClientIP:            ipOnly(parsed.ClientIP),
		ClientPrivateKey:    parsed.ClientPrivateKey,
		ClientPublicKey:     clientPub,
		PSKKey:              parsed.PresharedKey,
		ServerPublicKey:     parsed.ServerPublicKey,
		HostName:            host,
		Port:                port,
		PersistentKeepAlive: parsed.PersistentKeepalive,
		AllowedIPs:          parsed.AllowedIPs,
		Config:              in.Config,
	}

	lastCfgJSON, err := json.Marshal(lastCfg)
	if err != nil {
		return "", err
	}
	containerID := firstNonEmpty(strings.TrimSpace(in.DefaultContainerID), "amnezia-awg2")
	envelope := defaultVPNEnvelope{
		Containers: []defaultVPNContainer{{
			Container: containerID,
			AWG: defaultVPNAWG{
				Jc:              awg.Jc,
				Jmin:            awg.Jmin,
				Jmax:            awg.Jmax,
				S1:              awg.S1,
				S2:              awg.S2,
				S3:              awg.S3,
				S4:              awg.S4,
				H1:              awg.H1,
				H2:              awg.H2,
				H3:              awg.H3,
				H4:              awg.H4,
				I1:              awg.I1,
				I2:              awg.I2,
				I3:              awg.I3,
				I4:              awg.I4,
				I5:              awg.I5,
				Port:            port,
				ProtocolVersion: defaultInt(in.ProtocolVersion, 2),
				SubnetAddress:   subnetBase(firstNonEmpty(strings.TrimSpace(in.SubnetAddress), parsed.ClientIP)),
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

func normalizeAWG(in app.DefaultVPNAWGFields, parsed wgConfigParsed) app.DefaultVPNAWGFields {
	if in.Jc == 0 {
		in.Jc = parsed.Jc
	}
	if in.Jmin == 0 {
		in.Jmin = parsed.Jmin
	}
	if in.Jmax == 0 {
		in.Jmax = parsed.Jmax
	}
	if in.S1 == 0 {
		in.S1 = parsed.S1
	}
	if in.S2 == 0 {
		in.S2 = parsed.S2
	}
	if in.S3 == 0 {
		in.S3 = parsed.S3
	}
	if in.S4 == 0 {
		in.S4 = parsed.S4
	}
	in.H1 = firstNonEmpty(in.H1, parsed.H1)
	in.H2 = firstNonEmpty(in.H2, parsed.H2)
	in.H3 = firstNonEmpty(in.H3, parsed.H3)
	in.H4 = firstNonEmpty(in.H4, parsed.H4)
	in.I1 = firstNonEmpty(in.I1, parsed.I1)
	in.I2 = firstNonEmpty(in.I2, parsed.I2)
	in.I3 = firstNonEmpty(in.I3, parsed.I3)
	in.I4 = firstNonEmpty(in.I4, parsed.I4)
	in.I5 = firstNonEmpty(in.I5, parsed.I5)
	if in.MTU == 0 {
		in.MTU = parsed.MTU
	}
	return in
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
			case "clientid":
				out.ClientID = v
			case "mtu":
				fmt.Sscanf(v, "%d", &out.MTU)
			case "jc":
				fmt.Sscanf(v, "%d", &out.Jc)
			case "jmin":
				fmt.Sscanf(v, "%d", &out.Jmin)
			case "jmax":
				fmt.Sscanf(v, "%d", &out.Jmax)
			case "s1":
				fmt.Sscanf(v, "%d", &out.S1)
			case "s2":
				fmt.Sscanf(v, "%d", &out.S2)
			case "s3":
				fmt.Sscanf(v, "%d", &out.S3)
			case "s4":
				fmt.Sscanf(v, "%d", &out.S4)
			case "h1":
				out.H1 = v
			case "h2":
				out.H2 = v
			case "h3":
				out.H3 = v
			case "h4":
				out.H4 = v
			case "i1":
				out.I1 = v
			case "i2":
				out.I2 = v
			case "i3":
				out.I3 = v
			case "i4":
				out.I4 = v
			case "i5":
				out.I5 = v
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
				out.AllowedIPs = splitCSV(v)
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
	if len(out.AllowedIPs) == 0 {
		out.AllowedIPs = []string{"0.0.0.0/0", "::/0"}
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

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func ipOnly(v string) string {
	if strings.Contains(v, "/") {
		if pfx, err := netip.ParsePrefix(v); err == nil {
			return pfx.Addr().String()
		}
	}
	return strings.TrimSpace(v)
}

func subnetBase(v string) string {
	if strings.Contains(v, "/") {
		if pfx, err := netip.ParsePrefix(v); err == nil {
			return pfx.Masked().Addr().String()
		}
	}
	return strings.TrimSpace(v)
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

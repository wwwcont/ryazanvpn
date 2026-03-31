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
	"strconv"
	"strings"

	"github.com/wwwcont/ryazanvpn/internal/app"
	"github.com/wwwcont/ryazanvpn/internal/infra/wgkeys"
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
	Jc              string `json:"Jc"`
	Jmin            string `json:"Jmin"`
	Jmax            string `json:"Jmax"`
	S1              string `json:"S1"`
	S2              string `json:"S2"`
	S3              string `json:"S3"`
	S4              string `json:"S4"`
	H1              string `json:"H1"`
	H2              string `json:"H2"`
	H3              string `json:"H3"`
	H4              string `json:"H4"`
	I1              string `json:"I1"`
	I2              string `json:"I2"`
	I3              string `json:"I3"`
	I4              string `json:"I4"`
	I5              string `json:"I5"`
	Port            string `json:"port"`
	ProtocolVersion string `json:"protocol_version"`
	SubnetAddress   string `json:"subnet_address"`
	TransportProto  string `json:"transport_proto"`
	LastConfig      string `json:"last_config"`
}

type defaultVPNLastConfig struct {
	ClientID            string   `json:"clientId"`
	MTU                 string   `json:"mtu"`
	Jc                  string   `json:"Jc"`
	Jmin                string   `json:"Jmin"`
	Jmax                string   `json:"Jmax"`
	S1                  string   `json:"S1"`
	S2                  string   `json:"S2"`
	S3                  string   `json:"S3"`
	S4                  string   `json:"S4"`
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
	PersistentKeepAlive string   `json:"persistent_keep_alive"`
	AllowedIPs          []string `json:"allowed_ips"`
	Config              string   `json:"config"`
}

type wgConfigParsed struct {
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

	host := strings.TrimSpace(in.HostName)
	if host == "" {
		host = parsed.EndpointHost
	}
	port := in.Port
	if port <= 0 {
		port = parsed.EndpointPort
	}
	derivedPublicKey, err := wgkeys.DerivePublicKey(parsed.ClientPrivateKey)
	if err != nil {
		return "", fmt.Errorf("derive client public key: %w", err)
	}
	clientPublicKey := derivedPublicKey
	if provided := strings.TrimSpace(in.ClientPublicKey); provided != "" {
		if err := wgkeys.ValidateKeyPair(parsed.ClientPrivateKey, provided); err != nil {
			return "", fmt.Errorf("client key mismatch: provided public key does not match config private key")
		}
	}

	clientIP := stripCIDRSuffix(parsed.ClientIP)
	subnetAddress := deriveSubnetAddress(in.SubnetAddress, parsed.ClientIP)
	mtu := defaultInt(in.MTU, 1376)
	cfgText := firstNonEmpty(strings.TrimSpace(in.Config), "")

	lastCfg := defaultVPNLastConfig{
		ClientID:            clientPublicKey,
		MTU:                 strconv.Itoa(mtu),
		Jc:                  strconv.Itoa(in.AWG.Jc),
		Jmin:                strconv.Itoa(in.AWG.Jmin),
		Jmax:                strconv.Itoa(in.AWG.Jmax),
		S1:                  strconv.Itoa(in.AWG.S1),
		S2:                  strconv.Itoa(in.AWG.S2),
		S3:                  strconv.Itoa(in.AWG.S3),
		S4:                  strconv.Itoa(in.AWG.S4),
		H1:                  strings.TrimSpace(in.AWG.H1),
		H2:                  strings.TrimSpace(in.AWG.H2),
		H3:                  strings.TrimSpace(in.AWG.H3),
		H4:                  strings.TrimSpace(in.AWG.H4),
		I1:                  strings.TrimSpace(in.AWG.I1),
		I2:                  strings.TrimSpace(in.AWG.I2),
		I3:                  strings.TrimSpace(in.AWG.I3),
		I4:                  strings.TrimSpace(in.AWG.I4),
		I5:                  strings.TrimSpace(in.AWG.I5),
		ClientIP:            clientIP,
		ClientPrivateKey:    parsed.ClientPrivateKey,
		ClientPublicKey:     clientPublicKey,
		PSKKey:              parsed.PresharedKey,
		ServerPublicKey:     parsed.ServerPublicKey,
		HostName:            host,
		Port:                port,
		PersistentKeepAlive: strconv.Itoa(parsed.PersistentKeepalive),
		AllowedIPs:          parsed.AllowedIPs,
		Config:              cfgText,
	}
	lastCfgJSON, err := json.Marshal(lastCfg)
	if err != nil {
		return "", err
	}

	containerID := strings.TrimSpace(in.DefaultContainerID)
	if containerID == "" {
		containerID = "amnezia-awg2"
	}

	envelope := defaultVPNEnvelope{
		Containers: []defaultVPNContainer{{
			Container: containerID,
			AWG: defaultVPNAWG{
				Jc:              strconv.Itoa(in.AWG.Jc),
				Jmin:            strconv.Itoa(in.AWG.Jmin),
				Jmax:            strconv.Itoa(in.AWG.Jmax),
				S1:              strconv.Itoa(in.AWG.S1),
				S2:              strconv.Itoa(in.AWG.S2),
				S3:              strconv.Itoa(in.AWG.S3),
				S4:              strconv.Itoa(in.AWG.S4),
				H1:              strings.TrimSpace(in.AWG.H1),
				H2:              strings.TrimSpace(in.AWG.H2),
				H3:              strings.TrimSpace(in.AWG.H3),
				H4:              strings.TrimSpace(in.AWG.H4),
				I1:              strings.TrimSpace(in.AWG.I1),
				I2:              strings.TrimSpace(in.AWG.I2),
				I3:              strings.TrimSpace(in.AWG.I3),
				I4:              strings.TrimSpace(in.AWG.I4),
				I5:              strings.TrimSpace(in.AWG.I5),
				Port:            strconv.Itoa(port),
				ProtocolVersion: firstNonEmpty(strings.TrimSpace(fmt.Sprintf("%d", defaultInt(in.ProtocolVersion, 2))), "2"),
				SubnetAddress:   subnetAddress,
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
				out.AllowedIPs = parseCSV(v)
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

func parseCSV(v string) []string {
	items := strings.Split(v, ",")
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func stripCIDRSuffix(v string) string {
	v = strings.TrimSpace(v)
	ip, _, err := net.ParseCIDR(v)
	if err == nil {
		return ip.String()
	}
	return v
}

func deriveSubnetAddress(inSubnetAddress, clientCIDR string) string {
	if addr := strings.TrimSpace(inSubnetAddress); addr != "" {
		if ip, network, err := net.ParseCIDR(addr); err == nil {
			if network != nil {
				return network.IP.String()
			}
			return ip.String()
		}
		return stripCIDRSuffix(addr)
	}
	if ip, _, err := net.ParseCIDR(strings.TrimSpace(clientCIDR)); err == nil {
		v4 := ip.To4()
		if v4 != nil {
			return fmt.Sprintf("%d.%d.%d.0", v4[0], v4[1], v4[2])
		}
		return ip.String()
	}
	clientIP := stripCIDRSuffix(clientCIDR)
	parsed := net.ParseIP(clientIP)
	if v4 := parsed.To4(); v4 != nil {
		return fmt.Sprintf("%d.%d.%d.0", v4[0], v4[1], v4[2])
	}
	return clientIP
}

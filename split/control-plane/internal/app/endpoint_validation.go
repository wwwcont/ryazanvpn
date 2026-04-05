package app

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
)

var documentationCIDRs = mustPrefixes(
	"192.0.2.0/24",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"2001:db8::/32",
)

func ValidateConfigEndpointHost(rawHost string) error {
	host := strings.TrimSpace(rawHost)
	if host == "" {
		return fmt.Errorf("endpoint host is empty")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if isDocumentationIP(ip) {
			return fmt.Errorf("endpoint host uses documentation placeholder ip: %s", host)
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("endpoint host must be a public ip address: %s", host)
		}
		return nil
	}

	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" {
		return fmt.Errorf("endpoint host is empty")
	}
	if ip := net.ParseIP(host); ip != nil {
		return fmt.Errorf("invalid endpoint host format: %s", rawHost)
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".example") || strings.HasSuffix(host, ".invalid") || strings.HasSuffix(host, ".test") {
		return fmt.Errorf("endpoint host uses non-routable placeholder hostname: %s", rawHost)
	}
	return nil
}

func ValidateConfigEndpointPort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("endpoint port must be in range 1..65535, got %d", port)
	}
	return nil
}

func isDocumentationIP(ip netip.Addr) bool {
	for _, pfx := range documentationCIDRs {
		if pfx.Contains(ip) {
			return true
		}
	}
	return false
}

func mustPrefixes(values ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(values))
	for _, v := range values {
		pfx, err := netip.ParsePrefix(v)
		if err != nil {
			panic(err)
		}
		out = append(out, pfx)
	}
	return out
}

package nodeapi

import "testing"

func TestIsSupportedProtocolVersion(t *testing.T) {
	cases := []struct {
		name    string
		version string
		want    bool
	}{
		{name: "current", version: CurrentProtocolVersion, want: true},
		{name: "previous", version: PreviousProtocolVersion, want: true},
		{name: "unsupported", version: "-1", want: false},
		{name: "empty", version: "", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsSupportedProtocolVersion(tc.version); got != tc.want {
				t.Fatalf("IsSupportedProtocolVersion(%q)=%v want %v", tc.version, got, tc.want)
			}
		})
	}
}

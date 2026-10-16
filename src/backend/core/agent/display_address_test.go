// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import "testing"

func TestDisplayAddressPrefersTheAgentsOwnRoutableAddress(t *testing.T) {
	cases := []struct {
		name       string
		hostIP     string
		advertised []string
		observed   string
		want       string
	}{
		{
			name:       "skips loopback and takes the routable interface",
			advertised: []string{"http://127.0.0.1:8788", "http://10.35.36.254:8788"},
			observed:   "172.18.0.1",
			want:       "10.35.36.254",
		},
		{
			name:       "skips link-local",
			advertised: []string{"http://169.254.10.5:8788", "http://192.168.1.20:8788"},
			observed:   "172.18.0.1",
			want:       "192.168.1.20",
		},
		{
			name:       "an operator's hostname beats the gateway",
			advertised: []string{"https://agent.example.internal:8788"},
			observed:   "172.18.0.1",
			want:       "agent.example.internal",
		},
		{
			name:       "falls back to observed when nothing is usable",
			advertised: []string{"http://127.0.0.1:8788", "", "not a url"},
			observed:   "172.18.0.1",
			want:       "172.18.0.1",
		},
		{
			name:       "no candidates at all",
			advertised: nil,
			observed:   "203.0.113.9",
			want:       "203.0.113.9",
		},
		{
			name:       "the host's routed address beats a docker bridge it also carries",
			hostIP:     "10.35.229.32",
			advertised: []string{"http://172.18.0.1:8788", "http://10.35.229.32:8788"},
			observed:   "172.18.0.1",
			want:       "10.35.229.32",
		},
		{
			name:       "a loopback host ip is ignored in favour of a real interface",
			hostIP:     "127.0.0.1",
			advertised: []string{"http://192.168.5.4:8788"},
			observed:   "172.18.0.1",
			want:       "192.168.5.4",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DisplayAddress(tc.hostIP, tc.advertised, tc.observed); got != tc.want {
				t.Fatalf("DisplayAddress() = %q, want %q", got, tc.want)
			}
		})
	}
}

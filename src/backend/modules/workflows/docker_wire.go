// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"encoding"

	"github.com/moby/moby/api/types/container"
)

// wireText renders a netip or MAC value the way the Docker API sent it before
// the moby types: the zero value is "" rather than String()'s "invalid IP".
func wireText(v encoding.TextMarshaler) string {
	b, err := v.MarshalText()
	if err != nil {
		return ""
	}
	return string(b)
}

// wirePort keeps the JSON shape workflow nodes emitted for a container port
// before the moby migration, where an unbound IP was omitted rather than "".
type wirePort struct {
	IP          string `json:"IP,omitempty"`
	PrivatePort uint16 `json:"PrivatePort"`
	PublicPort  uint16 `json:"PublicPort,omitempty"`
	Type        string `json:"Type"`
}

// wirePorts converts container port summaries into the pre-migration shape.
func wirePorts(ports []container.PortSummary) []wirePort {
	if ports == nil {
		return nil
	}
	out := make([]wirePort, 0, len(ports))
	for _, p := range ports {
		out = append(out, wirePort{IP: wireText(p.IP), PrivatePort: p.PrivatePort, PublicPort: p.PublicPort, Type: p.Type})
	}
	return out
}

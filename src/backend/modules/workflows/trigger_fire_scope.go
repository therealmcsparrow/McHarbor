// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"fmt"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/nodelink"
)

// FireSourceHost is the fire source of a trigger the host itself raises; any other source
// is the slug of the plugin that fired.
const FireSourceHost = "host"

// hostFireActions is the closed list of action keys the host may fire through the plugin
// trigger path. A host key missing here cannot be fired, locally or forwarded.
var hostFireActions = map[string]bool{
	NodeITSMStatus: true,
}

// fireAllowed reports whether source may fire actionKey: the host only its named keys,
// a plugin only its own `plugin:<slug>:` keys. The leader applies the same rule to a
// forwarded fire, so a sending node cannot widen it.
func fireAllowed(source, actionKey string) error {
	source = strings.TrimSpace(source)
	switch {
	case source == "":
		return fmt.Errorf("%w: the fire names no source", nodelink.ErrFireNotAllowed)
	case source == FireSourceHost:
		if hostFireActions[actionKey] {
			return nil
		}
		return fmt.Errorf("%w: the host may not fire %q", nodelink.ErrFireNotAllowed, actionKey)
	case strings.ContainsAny(source, ": "):
		return fmt.Errorf("%w: %q is not a plugin slug", nodelink.ErrFireNotAllowed, source)
	case strings.HasPrefix(actionKey, "plugin:"+source+":") && len(actionKey) > len("plugin:"+source+":"):
		return nil
	}
	return fmt.Errorf("%w: plugin %s may not fire %q", nodelink.ErrFireNotAllowed, source, actionKey)
}

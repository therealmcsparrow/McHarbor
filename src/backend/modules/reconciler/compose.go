// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

// errComposeUnresolvable marks a compose file whose service set cannot be known
// without running compose itself. Such a stack is reported unknown, never drifted:
// a wrong expected set produces drift that never goes away, which costs more trust
// than the drift it would have caught.
var errComposeUnresolvable = errors.New("reconciler: compose file uses include; its services cannot be resolved here")

// composeServices returns the services a plain `docker compose up` starts, and
// every service the file declares. A service with profiles only starts when its
// profile is active, so it is declared but not expected.
func composeServices(content string) (expected, declared map[string]bool, err error) {
	var doc struct {
		Include  any                       `yaml:"include"`
		Services map[string]composeService `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return nil, nil, fmt.Errorf("parsing compose file: %w", err)
	}
	if doc.Include != nil {
		return nil, nil, errComposeUnresolvable
	}
	expected = make(map[string]bool, len(doc.Services))
	declared = make(map[string]bool, len(doc.Services))
	for name, svc := range doc.Services {
		declared[name] = true
		if len(svc.Profiles) == 0 {
			expected[name] = true
		}
	}
	return expected, declared, nil
}

type composeService struct {
	Profiles []string `yaml:"profiles"`
}

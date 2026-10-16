// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugincaps

import "testing"

// TestEstateSourceCardFieldsReachTheLoaderJSON holds the card fields to the registry value the
// frontend reads: LabelNS is copied from i18n_ns, which itself never reaches the JSON.
func TestEstateSourceCardFieldsReachTheLoaderJSON(t *testing.T) {
	caps, err := Parse("addon-demo", []byte(`
slug: addon-demo
adds_permissions:
  - key: demo.view
contributes:
  routes:
    - method: GET
      path: "/api/demo/machines"
      requires_permission: demo.view
  capabilities:
    - kind: estate-source
      metric: machines
      path: "/api/demo/machines"
      env_scoped: true
      label_key: estate.machines
      i18n_ns: demo
      icon: server
      running_field: state
      running_values: [running]
      link: "/plugins/addon-demo"
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c := caps[0]
	if c.LabelNS != "demo" || c.Icon != "server" || !c.EnvScoped || c.RunningField != "state" ||
		len(c.RunningValues) != 1 || c.Link != "/plugins/addon-demo" {
		t.Fatalf("card fields not carried: %+v", c)
	}
}

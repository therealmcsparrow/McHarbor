// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"testing"
)

// TestResourceFromPattern guards the plugin gate's resource match: the longest bound
// prefix wins, sub-routes inherit it, and the id is read from the parameter's segment.
func TestResourceFromPattern(t *testing.T) {
	RegisterResourceType(ResourceTypeInfo{Type: "t_vm", Source: "t-plugin", Routes: []ResourceRoute{{"/api/vms/{id}", "id"}}}, nil)
	RegisterResourceType(ResourceTypeInfo{Type: "t_share", Source: "t-plugin", Routes: []ResourceRoute{{"/api/vms/{id}/shares/{shareId}", "shareId"}}}, nil)
	t.Cleanup(func() { UnregisterResourceTypes("t-plugin") })
	cases := []struct{ pattern, path, typ, id string }{
		{"/api/vms/{id}", "/api/vms/vm1", "t_vm", "vm1"},
		{"/api/vms/{id}/memory", "/api/vms/vm1/memory", "t_vm", "vm1"},
		{"/api/vms/{id}/shares/{shareId}", "/api/vms/vm1/shares/s9", "t_share", "s9"},
		{"/api/vms", "/api/vms", "", ""},
		{"/api/vmsx/{id}", "/api/vmsx/a", "", ""},
	}
	for _, c := range cases {
		typ, id := ResourceFromPattern(c.pattern, c.path)
		if typ != c.typ || id != c.id {
			t.Errorf("%s -> %s/%s, want %s/%s", c.pattern, typ, id, c.typ, c.id)
		}
	}
	UnregisterResourceTypes("t-plugin")
	if typ, _ := ResourceFromPattern("/api/vms/{id}", "/api/vms/vm1"); typ != "" {
		t.Fatal("an unloaded plugin's types must stop matching")
	}
}

// TestPolicyHelpers pins the helpers and variables the editor documents, so the
// reference panel never promises a function the sandbox does not provide.
func TestPolicyHelpers(t *testing.T) {
	exec := NewABACExecutor(nil)
	attrs := &Attributes{
		User:     &UserAttributes{Roles: []string{"Operator"}, Groups: []string{"oncall"}, Labels: map[string]string{"team": "db"}},
		Resource: &ResourceAttributes{Name: "web-12", Labels: map[string]string{"tier": "api", "team": "db"}, Attrs: map[string]any{"vcpus": 16}},
		Request:  &RequestAttributes{IP: "10.2.3.4", APIKey: true, ReadOnly: false},
		Now:      &TimeAttributes{Hour: 9, Weekday: 6},
	}
	for expr, want := range map[string]bool{
		`hasRole("Operator") && !hasRole("Admin")`:                           true,
		`inGroup("oncall")`:                                                  true,
		`labelIn(resource.labels, "tier", "web", "api")`:                     true,
		`matches(resource.name, "^web-[0-9]+$")`:                             true,
		`startsWith(resource.name, "web") && endsWith(resource.name, "12")`:  true,
		`contains(resource.name, "b-1")`:                                     true,
		`inCIDR(request.ip, "192.168.0.0/16", "10.0.0.0/8")`:                 true,
		`inCIDR(request.ip, "192.168.0.0/16")`:                               false,
		`between(now.hour, 8, 18)`:                                           true,
		`isWeekend()`:                                                        true,
		`request.apiKey && !request.readOnly`:                                true,
		`resource.attrs.vcpus > 8`:                                           true,
		`hasLabel(resource.labels, "team") == hasLabel(user.labels, "team")`: true,
	} {
		res := exec.Evaluate(context.Background(), expr, attrs)
		if res.Err != nil || res.Matched != want {
			t.Errorf("%s = %v (err %v), want %v", expr, res.Matched, res.Err, want)
		}
	}
	for _, h := range PolicyHelpers() {
		if res := exec.Evaluate(context.Background(), h.Example, attrs); res.Err != nil {
			t.Errorf("documented example for %s does not run: %v", h.Name, res.Err)
		}
	}
}

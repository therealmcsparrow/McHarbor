// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package netconf

import (
	"errors"
	"strings"
	"testing"
)

func TestHostLinkAliasIsOwnerScoped(t *testing.T) {
	if got := HostLinkAlias(HostLinkOwnerDocker, "abc123"); got != "mcharbor:hostlink:docker:abc123" {
		t.Fatalf("alias = %q", got)
	}
	if HostLinkAlias(HostLinkOwnerCompute, "x") == HostLinkAlias(HostLinkOwnerDocker, "x") {
		t.Fatal("a compute and a docker link with the same id must not share an alias")
	}
	if !strings.HasPrefix(HostLinkAlias(HostLinkOwnerCompute, "x"), HostLinkAliasPrefix) {
		t.Fatal("the alias must carry the prefix Links() and deleteAliased recognise")
	}
}

func TestValidateHostLinkName(t *testing.T) {
	const ownerID = "cr1n2b3c4d5e6f7g8h9i"
	cases := []struct {
		name string
		ok   bool
	}{
		{"dockerlan0", true},
		{"lan.100", true},
		{"a", true},
		{"abcdefghijklmno", true},
		{"abcdefghijklmnop", false}, // 16 characters: over the kernel's limit
		{"", false},
		{"-lead", false},
		{".hidden", false},
		{"has space", false},
		{"slash/name", false},
		{"colon:name", false},
		{"mcharbor-xyz", false}, // managed prefix, not this owner's generated name
		{"mchd12345678", false},
		{HostShimName(ownerID), true},
	}
	for _, c := range cases {
		err := ValidateHostLinkName(c.name, ownerID)
		if (err == nil) != c.ok {
			t.Errorf("ValidateHostLinkName(%q) = %v, want ok=%v", c.name, err, c.ok)
		}
		if err != nil && !errors.Is(err, ErrHostLinkInvalid) {
			t.Errorf("ValidateHostLinkName(%q) error %v is not ErrHostLinkInvalid", c.name, err)
		}
	}
}

func TestResolveHostShimDefaultsToTheComputeShim(t *testing.T) {
	spec := ResolveHostShim(HostShimSpec{NetworkID: "cr1n2b3c4d5e6f7g8h9i"})
	if spec.Owner != HostLinkOwnerCompute || spec.OwnerID != "cr1n2b3c4d5e6f7g8h9i" {
		t.Fatalf("owner = %s/%s", spec.Owner, spec.OwnerID)
	}
	if spec.Name != HostShimName("cr1n2b3c4d5e6f7g8h9i") {
		t.Fatalf("name = %q, want the pre-alias generated name", spec.Name)
	}
	named := ResolveHostShim(HostShimSpec{NetworkID: "n1", Name: "lan0"})
	if named.Name != "lan0" {
		t.Fatalf("an operator name was replaced: %q", named.Name)
	}
}

func TestValidateHostShimChecksAddressFamilies(t *testing.T) {
	base := HostShimSpec{Owner: HostLinkOwnerDocker, OwnerID: "d1", Name: "lan0", Parent: "eth0"}
	cases := []struct {
		label string
		edit  func(*HostShimSpec)
		ok    bool
	}{
		{"plain", func(*HostShimSpec) {}, true},
		{"v4", func(s *HostShimSpec) { s.AddressCIDR = "192.168.1.5/24" }, true},
		{"v6 in v4 field", func(s *HostShimSpec) { s.AddressCIDR = "fd00::5/64" }, false},
		{"v4 in v6 field", func(s *HostShimSpec) { s.IPv6Enabled = true; s.AddressCIDR6 = "192.168.1.5/24" }, false},
		{"v6 static", func(s *HostShimSpec) { s.IPv6Enabled = true; s.AddressCIDR6 = "fd00::5/64" }, true},
		{"v6 without enable", func(s *HostShimSpec) { s.AddressCIDR6 = "fd00::5/64" }, false},
		{"slaac", func(s *HostShimSpec) { s.IPv6Enabled = true }, true},
		{"bare ip", func(s *HostShimSpec) { s.AddressCIDR = "192.168.1.5" }, false},
		{"unknown owner", func(s *HostShimSpec) { s.Owner = "k8s" }, false},
		{"owner id with colon", func(s *HostShimSpec) { s.OwnerID = "a:b" }, false},
		{"named after parent", func(s *HostShimSpec) { s.Name = "eth0" }, false},
		{"no parent", func(s *HostShimSpec) { s.Parent = "" }, false},
	}
	for _, c := range cases {
		spec := base
		c.edit(&spec)
		err := ValidateHostShim(spec)
		if (err == nil) != c.ok {
			t.Errorf("%s: ValidateHostShim = %v, want ok=%v", c.label, err, c.ok)
		}
		if err != nil && !errors.Is(err, ErrHostLinkInvalid) {
			t.Errorf("%s: error %v is not ErrHostLinkInvalid", c.label, err)
		}
	}
}

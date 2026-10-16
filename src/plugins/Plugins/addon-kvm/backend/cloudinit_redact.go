// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/therealmcsparrow/mcharbor/core/envredact"
)

// codeUserDataMasked refuses a create whose user-data still carries the redaction marker:
// the guest would boot with bullets where a credential was meant to be.
const codeUserDataMasked = "err.vm.user_data_masked"

// inlineAssignment finds `NAME=value` and `name: value` pairs inside free text — a runcmd
// line, a write_files body — so a credential there is withheld by its name like an env value.
var inlineAssignment = regexp.MustCompile(`(?m)(^|[\s"'])([A-Za-z_][A-Za-z0-9_.-]*)(=|:[ \t]+)([^\s"']+)`)

// userDataPolicy is the classifier the viewer sees user-data through; the reveal route
// returns the stored text instead, so there is no permitted variant here.
var userDataPolicy = envredact.Policy{}

// maskUserData returns the operator's extra cloud-config with credential-looking values
// replaced by envredact.Marker. Everything under a credential-named key (`passwd`,
// `chpasswd`, a `*_token`) is withheld whole; other strings are scanned for assignments.
func maskUserData(userData string) string {
	if strings.TrimSpace(userData) == "" {
		return userData
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(userData), &doc); err != nil || len(doc.Content) == 0 {
		return maskText(userData)
	}
	if !maskNode(&doc, false) {
		return userData
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		// Unencodable after an edit: fall back to the line scan, never to the original.
		return maskText(userData)
	}
	return string(out)
}

// maskNode masks n in place and reports whether it changed anything. sensitive is true when
// an enclosing key already names a credential, so every scalar below it is withheld.
func maskNode(n *yaml.Node, sensitive bool) bool {
	changed := false
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, child := range n.Content {
			changed = maskNode(child, sensitive) || changed
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i].Value
			changed = maskNode(n.Content[i+1], sensitive || envredact.SensitiveName(key)) || changed
		}
	case yaml.ScalarNode:
		if n.Value == "" || n.Value == envredact.Marker {
			return false
		}
		// A whole-value check would mask a write_files body for one line in it; the inline
		// pass masks only the credential and leaves the rest readable.
		masked := envredact.Marker
		if !sensitive {
			masked = maskText(n.Value)
		}
		if masked != n.Value {
			n.Value, n.Style, n.Tag = masked, 0, "!!str"
			changed = true
		}
	}
	return changed
}

// maskText withholds every credential in s — the classifier's inline shapes, then any
// assignment with a credential-looking name — leaving the rest of the text as it was. It is
// also the whole rule for user-data that is not YAML.
func maskText(s string) string {
	s = userDataPolicy.MaskInline(s)
	return inlineAssignment.ReplaceAllStringFunc(s, func(match string) string {
		parts := inlineAssignment.FindStringSubmatch(match)
		if !userDataPolicy.Withhold(parts[2], parts[4]) {
			return match
		}
		return parts[1] + parts[2] + parts[3] + envredact.Marker
	})
}

// withheldVM is vm as a viewer may see it: its cloud-init user-data masked. Service.Get's own
// result is left untouched, because the seed builder and clone read the stored text from it.
func withheldVM(vm VM) VM {
	if vm.CloudInit != nil {
		ci := *vm.CloudInit
		ci.UserDataExtra = maskUserData(ci.UserDataExtra)
		vm.CloudInit = &ci
	}
	return vm
}

// userDataCarriesMarker reports whether a create request would store the redaction marker.
func userDataCarriesMarker(req CreateRequest) bool {
	return req.CloudInit != nil && strings.Contains(req.CloudInit.UserDataExtra, envredact.Marker)
}

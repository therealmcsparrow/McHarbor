// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package procenv builds the environments McHarbor hands to the processes it starts. A
// compose run interpolates ${ANY_VARIABLE} from its own environment into images, labels and
// error messages, so it gets an allow-list rather than McHarbor's whole environment.
package procenv

import (
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/envredact"
)

// childNames are the variables a docker or compose child needs to find its binary, its
// config, its daemon and the network; everything else McHarbor runs with stays behind.
// Names are compared upper-cased, which also admits Windows' mixed-case spellings.
var childNames = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "SHELL": true, "TERM": true,
	"LANG": true, "LANGUAGE": true, "TZ": true, "TMPDIR": true, "TMP": true, "TEMP": true,
	"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true, "ALL_PROXY": true,
	"SSL_CERT_FILE": true, "SSL_CERT_DIR": true, "SSH_AUTH_SOCK": true,
	"XDG_CONFIG_HOME": true, "XDG_RUNTIME_DIR": true, "XDG_CACHE_HOME": true,
	"SYSTEMROOT": true, "WINDIR": true, "COMSPEC": true, "PATHEXT": true, "USERPROFILE": true,
	"APPDATA": true, "LOCALAPPDATA": true, "PROGRAMDATA": true, "PROGRAMFILES": true,
	"PROGRAMFILES(X86)": true, "PROGRAMW6432": true, "SYSTEMDRIVE": true,
}

// childPrefixes are variable families the docker CLI and compose read for their own
// behaviour: daemon selection and TLS, compose options, BuildKit, and locale.
var childPrefixes = []string{"DOCKER_", "COMPOSE_", "BUILDKIT_", "LC_"}

// Child returns the allow-listed part of environ for a docker or compose child process.
// A root-of-trust variable never passes, whatever its name happens to match.
func Child(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || isRootOfTrust(name) {
			continue
		}
		if allowedChild(strings.ToUpper(name)) {
			out = append(out, entry)
		}
	}
	return out
}

func allowedChild(upper string) bool {
	if childNames[upper] {
		return true
	}
	for _, prefix := range childPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
}

// WithoutRootOfTrust returns environ without McHarbor's key variables and the _FILE paths
// that point at them, for a helper that runs McHarbor's own binary and needs the rest.
func WithoutRootOfTrust(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if isRootOfTrust(name) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func isRootOfTrust(name string) bool {
	return envredact.RootOfTrustName(name) || envredact.RootOfTrustName(strings.TrimSuffix(name, "_FILE"))
}

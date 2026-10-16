// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"net"
	"regexp"
	"strings"

	"github.com/dop251/goja"
)

// PolicyHelper documents one function a policy expression can call, for the editor.
type PolicyHelper struct {
	Name        string `json:"name"`
	Signature   string `json:"signature"`
	Description string `json:"description"`
	Example     string `json:"example"`
}

// maxPattern bounds a matches() pattern: RE2 is linear-time, but a huge pattern still
// costs compile time inside the 50 ms budget.
const maxPattern = 256

// PolicyHelpers lists every helper, in the order the editor shows them.
func PolicyHelpers() []PolicyHelper {
	return []PolicyHelper{
		{"hasLabel", "hasLabel(labels, key)", "The label's value, or an empty string when it is absent.", `hasLabel(resource.labels, "env") == "prod"`},
		{"labelIn", "labelIn(labels, key, value, ...)", "Whether the label is one of the values.", `labelIn(resource.labels, "tier", "web", "api")`},
		{"inList", "inList(value, item, ...)", "Whether value equals any of the items.", `inList(env.name, "prod", "prod-eu")`},
		{"hasRole", "hasRole(name)", "Whether the user holds a role with this name.", `hasRole("Platform Admin")`},
		{"inGroup", "inGroup(name)", "Whether the user is a member of this group.", `inGroup("oncall")`},
		{"startsWith", "startsWith(text, prefix)", "Prefix test.", `startsWith(resource.name, "db-")`},
		{"endsWith", "endsWith(text, suffix)", "Suffix test.", `endsWith(user.email, "@example.com")`},
		{"contains", "contains(text, part)", "Substring test.", `contains(resource.name, "staging")`},
		{"matches", "matches(text, pattern)", "Regular expression test (RE2 syntax, 256 characters at most).", `matches(resource.name, "^web-[0-9]+$")`},
		{"inCIDR", "inCIDR(ip, cidr, ...)", "Whether the IP lies in any of the networks.", `inCIDR(request.ip, "10.0.0.0/8", "192.168.0.0/16")`},
		{"between", "between(number, low, high)", "Inclusive range test.", `between(now.hour, 8, 18)`},
		{"isWeekend", "isWeekend()", "Saturday or Sunday, UTC.", `!isWeekend() && between(now.hour, 8, 18)`},
	}
}

// PolicyVariables lists the non-resource variables, for the editor's reference panel.
func PolicyVariables() []ResourceAttribute {
	return []ResourceAttribute{
		{Path: "user.id", Type: "string", Description: "Caller's user id"},
		{Path: "user.username", Type: "string", Description: "Caller's username"},
		{Path: "user.email", Type: "string", Description: "Caller's email address"},
		{Path: "user.roles", Type: "list", Description: "Names of every role the caller holds, directly or through groups"},
		{Path: "user.groups", Type: "list", Description: "Names of the caller's groups"},
		{Path: "user.labels", Type: "map", Description: "Labels set on the user; read with hasLabel(user.labels, key)"},
		{Path: "env.id", Type: "string", Description: "Environment of the request, empty for global actions"},
		{Path: "env.name", Type: "string", Description: "Environment name"},
		{Path: "env.type", Type: "string", Description: "Connection type: socket, tcp, agent …"},
		{Path: "request.method", Type: "string", Description: "HTTP method"},
		{Path: "request.path", Type: "string", Description: "Route path"},
		{Path: "request.ip", Type: "string", Description: "Client address, honouring trusted proxies"},
		{Path: "request.apiKey", Type: "bool", Description: "The call was made with an API key"},
		{Path: "request.readOnly", Type: "bool", Description: "GET, HEAD or OPTIONS"},
		{Path: "now.hour", Type: "number", Description: "Hour 0–23, server time"},
		{Path: "now.minute", Type: "number", Description: "Minute 0–59"},
		{Path: "now.weekday", Type: "number", Description: "0 = Sunday … 6 = Saturday"},
		{Path: "now.day", Type: "number", Description: "Day of the month"},
		{Path: "now.month", Type: "number", Description: "Month 1–12"},
		{Path: "now.year", Type: "number", Description: "Year"},
		{Path: "now.date", Type: "string", Description: "YYYY-MM-DD, UTC"},
		{Path: "now.unixTime", Type: "number", Description: "Seconds since 1970"},
	}
}

func installHelpers(rt *goja.Runtime, attrs Attributes) error {
	str := func(v goja.Value) string {
		if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
			return ""
		}
		return v.String()
	}
	has := func(list []string, want string) bool {
		for _, v := range list {
			if v == want {
				return true
			}
		}
		return false
	}
	helpers := map[string]func(goja.FunctionCall) goja.Value{
		"labelIn": func(c goja.FunctionCall) goja.Value {
			m, _ := c.Argument(0).Export().(map[string]string)
			v, ok := m[str(c.Argument(1))]
			if !ok {
				return rt.ToValue(false)
			}
			for i := 2; i < len(c.Arguments); i++ {
				if v == str(c.Argument(i)) {
					return rt.ToValue(true)
				}
			}
			return rt.ToValue(false)
		},
		"hasRole": func(c goja.FunctionCall) goja.Value {
			return rt.ToValue(has(attrs.User.Roles, str(c.Argument(0))))
		},
		"inGroup": func(c goja.FunctionCall) goja.Value {
			return rt.ToValue(has(attrs.User.Groups, str(c.Argument(0))))
		},
		"startsWith": func(c goja.FunctionCall) goja.Value {
			return rt.ToValue(strings.HasPrefix(str(c.Argument(0)), str(c.Argument(1))))
		},
		"endsWith": func(c goja.FunctionCall) goja.Value {
			return rt.ToValue(strings.HasSuffix(str(c.Argument(0)), str(c.Argument(1))))
		},
		"contains": func(c goja.FunctionCall) goja.Value {
			return rt.ToValue(strings.Contains(str(c.Argument(0)), str(c.Argument(1))))
		},
		"matches": func(c goja.FunctionCall) goja.Value {
			pattern := str(c.Argument(1))
			if len(pattern) > maxPattern {
				panic(rt.NewTypeError("matches: pattern longer than 256 characters")) // panic-ok: goja converts this panic into a JS TypeError thrown to the ABAC script
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				panic(rt.NewTypeError("matches: " + err.Error())) // panic-ok: goja converts this panic into a JS TypeError thrown to the ABAC script
			}
			return rt.ToValue(re.MatchString(str(c.Argument(0))))
		},
		"inCIDR": func(c goja.FunctionCall) goja.Value {
			ip := net.ParseIP(str(c.Argument(0)))
			if ip == nil {
				return rt.ToValue(false)
			}
			for i := 1; i < len(c.Arguments); i++ {
				if _, n, err := net.ParseCIDR(str(c.Argument(i))); err == nil && n.Contains(ip) {
					return rt.ToValue(true)
				}
			}
			return rt.ToValue(false)
		},
		"between": func(c goja.FunctionCall) goja.Value {
			n, lo, hi := c.Argument(0).ToFloat(), c.Argument(1).ToFloat(), c.Argument(2).ToFloat()
			return rt.ToValue(n >= lo && n <= hi)
		},
		"isWeekend": func(goja.FunctionCall) goja.Value {
			return rt.ToValue(attrs.Now.Weekday == 0 || attrs.Now.Weekday == 6)
		},
	}
	for name, fn := range helpers {
		if err := rt.Set(name, fn); err != nil {
			return err
		}
	}
	return nil
}

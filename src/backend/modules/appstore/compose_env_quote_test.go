// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package appstore

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const awkwardPassword = `p"a\ss: #1`

func TestRenderComposeEnvPlaceholdersKeepsMapValuesValidYAML(t *testing.T) {
	compose := strings.Join([]string{
		"services:",
		"  app:",
		"    environment:",
		`      DATABASE_URL: "postgres://user:{{env.DB_PASSWORD}}@db:5432/app"`,
		"      PLAIN: {{env.DB_PASSWORD}}",
	}, "\n")
	rendered := renderComposeEnvPlaceholders(compose, map[string]string{"DB_PASSWORD": awkwardPassword})

	var doc struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &doc); err != nil {
		t.Fatalf("rendered compose is not valid YAML: %v\n%s", err, rendered)
	}
	env := doc.Services["app"].Environment
	if want := "postgres://user:" + awkwardPassword + "@db:5432/app"; env["DATABASE_URL"] != want {
		t.Fatalf("quoted value = %q, want %q", env["DATABASE_URL"], want)
	}
	if env["PLAIN"] != awkwardPassword {
		t.Fatalf("bare value = %q, want %q", env["PLAIN"], awkwardPassword)
	}
}

func TestRenderComposeEnvPlaceholdersKeepsListItemsValidYAML(t *testing.T) {
	compose := strings.Join([]string{
		"services:",
		"  app:",
		"    environment:",
		"      - LISTED={{env.DB_PASSWORD}}",
	}, "\n")
	rendered := renderComposeEnvPlaceholders(compose, map[string]string{"DB_PASSWORD": awkwardPassword})

	var doc struct {
		Services map[string]struct {
			Environment []string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &doc); err != nil {
		t.Fatalf("rendered compose is not valid YAML: %v\n%s", err, rendered)
	}
	if got, want := doc.Services["app"].Environment, "LISTED="+awkwardPassword; len(got) != 1 || got[0] != want {
		t.Fatalf("list item = %q, want %q", got, want)
	}
}

func TestRenderComposeOverrideExpandsIdentityTokensInEnvValues(t *testing.T) {
	app := AppTemplate{
		Image: "example/app:1",
		ComposeOverride: strings.Join([]string{
			"services:",
			"  {{serviceName}}:",
			"    image: {{image}}",
			"    environment:",
			"      REDIS_HOST: {{env.REDIS_HOST}}",
			"      MTA_HOST: {{env.MTA_HOST}}",
			"      NESTED: {{env.NESTED}}",
		}, "\n"),
	}
	envVars := map[string]string{
		"REDIS_HOST": "{{serviceName}}-redis",
		"MTA_HOST":   "{{containerName}}-core",
		"NESTED":     "{{env.REDIS_HOST}}",
	}
	rendered := renderComposeOverride(app, "shop", nil, nil, envVars, nil)

	if strings.Contains(rendered, "{{serviceName}}") || strings.Contains(rendered, "{{containerName}}") {
		t.Fatalf("identity placeholder survived rendering:\n%s", rendered)
	}
	var doc struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &doc); err != nil {
		t.Fatalf("rendered compose is not valid YAML: %v\n%s", err, rendered)
	}
	env := doc.Services["shop"].Environment
	if env["REDIS_HOST"] != "shop-redis" || env["MTA_HOST"] != "shop-core" {
		t.Fatalf("env = %#v, want shop-redis and shop-core", env)
	}
	if env["NESTED"] != "{{env.REDIS_HOST}}" {
		t.Fatalf("nested env placeholder = %q, want it left literal so rendering stays one bounded pass", env["NESTED"])
	}
}

func TestExpandEnvIdentityTokensDoesNotRescanItsOutput(t *testing.T) {
	out := expandEnvIdentityTokens(map[string]string{"A": "{{serviceName}}"}, "{{serviceName}}", "/x")
	if out["A"] != "{{serviceName}}" {
		t.Fatalf("A = %q, want a single pass", out["A"])
	}
}

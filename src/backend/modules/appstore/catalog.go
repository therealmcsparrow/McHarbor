// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package appstore serves the app catalog and installs catalog apps as Compose stacks.
package appstore

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// loadBundledCatalog remains for catalog validation tests. Runtime catalog
// loading is registry-driven and never calls this compatibility helper.
func loadBundledCatalog() (*CatalogFile, error) {
	appsDir, err := findProjectAppCatalogDir()
	if err != nil {
		return nil, err
	}
	return loadCatalogDirectory(appsDir)
}

func loadCatalogDirectory(appsDir string) (*CatalogFile, error) {
	entries, err := os.ReadDir(appsDir)
	if err != nil {
		return nil, fmt.Errorf("reading app catalog directory: %w", err)
	}

	files := make([]string, 0, len(entries))
	var totalBytes int64
	for _, entry := range entries {
		// index.json is the published catalog's single-document form of these same files
		// (scripts/apps/build-index.mjs); read as an app it would fail the slug check.
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" || entry.Name() == "index.json" {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("catalog app %s must not be a symlink", entry.Name())
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("inspecting catalog app %s: %w", entry.Name(), err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("catalog app %s must be a regular file", entry.Name())
		}
		if info.Size() > maxCatalogFileBytes {
			return nil, fmt.Errorf("catalog app %s exceeds %d bytes", entry.Name(), maxCatalogFileBytes)
		}
		totalBytes += info.Size()
		if totalBytes > maxLocalCatalogBytes {
			return nil, fmt.Errorf("app catalog exceeds %d bytes", maxLocalCatalogBytes)
		}
		files = append(files, filepath.Join(appsDir, entry.Name()))
		if len(files) > maxCatalogFiles {
			return nil, fmt.Errorf("app catalog exceeds %d files", maxCatalogFiles)
		}
	}
	sort.Strings(files)

	catalog := &CatalogFile{Version: "1.0.0", Apps: make([]AppTemplate, 0, len(files))}
	for _, file := range files {
		app, err := loadAppTemplateFile(file)
		if err != nil {
			return nil, err
		}
		catalog.Apps = append(catalog.Apps, app)
	}

	return catalog, nil
}

func findProjectAppCatalogDir() (string, error) {
	candidates := []string{
		"mnt/app-catalog",
		"../../mnt/app-catalog",
		"../../../mnt/app-catalog",
		"../../../../mnt/app-catalog",
	}

	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "mnt", "app-catalog"))
	}

	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("project app catalog directory not found")
}

func loadAppTemplateFile(path string) (AppTemplate, error) {
	file, err := os.Open(path)
	if err != nil {
		return AppTemplate{}, fmt.Errorf("reading catalog app %s: %w", filepath.Base(path), err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Warn("app catalog: closing catalog file failed", "file", filepath.Base(path), "error", err)
		}
	}()

	data, err := io.ReadAll(io.LimitReader(file, maxCatalogFileBytes+1))
	if err != nil {
		return AppTemplate{}, fmt.Errorf("reading catalog app %s: %w", filepath.Base(path), err)
	}
	if len(data) > maxCatalogFileBytes {
		return AppTemplate{}, fmt.Errorf("catalog app %s exceeds %d bytes", filepath.Base(path), maxCatalogFileBytes)
	}

	var app AppTemplate
	if err := json.Unmarshal(data, &app); err != nil {
		return AppTemplate{}, fmt.Errorf("parsing catalog app %s: %w", filepath.Base(path), err)
	}
	if app.Slug == "" {
		return AppTemplate{}, fmt.Errorf("catalog app %s has empty slug", filepath.Base(path))
	}
	expectedSlug := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if app.Slug != expectedSlug {
		return AppTemplate{}, fmt.Errorf("catalog app %s has mismatched slug %q", filepath.Base(path), app.Slug)
	}
	if !appSlugPattern.MatchString(app.Slug) {
		return AppTemplate{}, fmt.Errorf("catalog app %s has invalid slug", filepath.Base(path))
	}

	return app, nil
}

// installImage is the image reference an install actually pulls. An app whose
// catalog entry carries a "latest" block installs the release line, not the tag
// the catalog happens to pin: the block exists because the catalog server tracks
// what "newest" means for this app, so the install follows it. Only a release tag
// is rewritten: a channel tag (stable, nightly, alpine) is the line the catalog
// chose, and many images (frigate) publish no :latest at all. A digest is left
// alone — it pins bytes on purpose, and rewriting it would discard the pin.
func installImage(app AppTemplate) string {
	image := strings.TrimSpace(app.Image)
	if app.Latest == nil || image == "" || strings.Contains(image, "@") {
		return app.Image
	}

	idx := strings.LastIndex(image, ":")
	if idx <= 0 || strings.Contains(image[idx+1:], "/") {
		return app.Image
	}
	if _, release := classifyTag(image[idx+1:]); !release {
		return app.Image
	}
	return image[:idx] + ":latest"
}

// latestVersion and latestReleaseDate flatten the optional block for storage.
// A catalog that emits no block stores two empty strings, which scanCatalogApp
// reads back as a nil Latest.
func latestVersion(app AppTemplate) string {
	if app.Latest == nil {
		return ""
	}
	return app.Latest.Version
}

func latestReleaseDate(app AppTemplate) string {
	if app.Latest == nil {
		return ""
	}
	return app.Latest.ReleaseDate
}

// generateCompose builds a docker-compose.yml string from an AppTemplate
// with optional user overrides for ports, volumes, env vars, and networking.
func generateCompose(app AppTemplate, name string, ports []PortMapping, volumes []VolumeMount, envVars map[string]string, netCfg *NetworkConfig) string {
	serviceName := sanitizeServiceName(name)
	if app.ComposeOverride != "" {
		return renderComposeOverride(app, serviceName, ports, volumes, envVars, netCfg)
	}

	var b strings.Builder
	b.WriteString("# Generated by McHarbor App Store\n")
	b.WriteString("services:\n")
	fmt.Fprintf(&b, "  %s:\n", serviceName)
	fmt.Fprintf(&b, "    image: %s\n", app.Image)
	fmt.Fprintf(&b, "    container_name: %s\n", serviceName)
	b.WriteString("    restart: unless-stopped\n")

	// Ports
	if len(ports) > 0 {
		b.WriteString("    ports:\n")
		for _, p := range ports {
			proto := p.Protocol
			if proto == "" {
				proto = "tcp"
			}
			if proto == "tcp" {
				fmt.Fprintf(&b, "      - \"%d:%d\"\n", p.Host, p.Container)
			} else {
				fmt.Fprintf(&b, "      - \"%d:%d/%s\"\n", p.Host, p.Container, proto)
			}
		}
	}

	// Volumes
	if len(volumes) > 0 {
		b.WriteString("    volumes:\n")
		for _, v := range volumes {
			suffix := ""
			if v.ReadOnly {
				suffix = ":ro"
			}
			fmt.Fprintf(&b, "      - %s:%s%s\n", v.Host, v.Container, suffix)
		}
	}

	// Environment variables
	if len(envVars) > 0 {
		b.WriteString("    environment:\n")
		for k, v := range envVars {
			fmt.Fprintf(&b, "      - %s\n", yamlEnvItem(k, v))
		}
	}

	renderGeneratedNetwork(&b, netCfg)

	return b.String()
}

var composeEnvPlaceholder = regexp.MustCompile(`\{\{env\.([A-Za-z_][A-Za-z0-9_]*)\}\}`)
var composeQuotedEnvValue = regexp.MustCompile(`^\s*(?:-\s*)?[^\s:"]+:\s+"[^"]*\{\{env\.[A-Za-z_][A-Za-z0-9_]*\}\}.*"\s*$`)
var composeEnvListItemPlaceholder = regexp.MustCompile(`^(\s*-\s*)\{\{env\.([A-Za-z_][A-Za-z0-9_]*)\}\}(.*)$`)
var composeEnvListItemInline = regexp.MustCompile(`^(\s*-\s*)([^"'\s][^"']*)$`)
var composeRawEnvPlaceholder = regexp.MustCompile(`\{\{envRaw\.([A-Za-z_][A-Za-z0-9_]*)\}\}`)
var appSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func renderComposeOverride(app AppTemplate, serviceName string, ports []PortMapping, volumes []VolumeMount, envVars map[string]string, netCfg *NetworkConfig) string {
	compose := app.ComposeOverride
	appDataPath := "/opt/appdata/" + serviceName
	envVars = expandEnvIdentityTokens(envVars, serviceName, appDataPath)
	replacements := map[string]string{
		"{{serviceName}}":    serviceName,
		"{{containerName}}":  serviceName,
		"{{projectName}}":    serviceName,
		"{{appDataPath}}":    appDataPath,
		"{{image}}":          installImage(app),
		"{{ports}}":          renderComposePorts(ports),
		"{{volumes}}":        renderComposeVolumes(volumes),
		"{{environment}}":    renderComposeEnvironment(envVars),
		"{{environmentMap}}": renderComposeEnvironmentMap(envVars),
		"{{networks}}":       renderComposeNetwork(netCfg),
		"{{networkMode}}":    renderComposeNetworkMode(netCfg),
	}
	for placeholder, value := range replacements {
		compose = strings.ReplaceAll(compose, placeholder, value)
	}
	compose = renderComposeEnvPlaceholders(compose, envVars)
	compose = composeRawEnvPlaceholder.ReplaceAllStringFunc(compose, func(match string) string {
		key := composeRawEnvPlaceholder.FindStringSubmatch(match)[1]
		return envVars[key]
	})
	if !strings.HasSuffix(compose, "\n") {
		compose += "\n"
	}
	return compose
}

// expandEnvIdentityTokens returns a copy of envVars with the identity placeholders expanded, so a catalog
// default or an operator-typed value such as "{{serviceName}}-redis" reaches the compose file resolved.
// It is a single non-rescanning pass, which keeps it bounded because the substituted values never hold "{{".
func expandEnvIdentityTokens(envVars map[string]string, serviceName, appDataPath string) map[string]string {
	if len(envVars) == 0 {
		return envVars
	}
	replacer := strings.NewReplacer(
		"{{serviceName}}", serviceName,
		"{{containerName}}", serviceName,
		"{{projectName}}", serviceName,
		"{{appDataPath}}", appDataPath,
	)
	out := make(map[string]string, len(envVars))
	for k, v := range envVars {
		out[k] = replacer.Replace(v)
	}
	return out
}

func renderComposeEnvPlaceholders(compose string, envVars map[string]string) string {
	lines := strings.Split(compose, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "{{env.") {
			continue
		}

		if match := composeEnvListItemPlaceholder.FindStringSubmatch(line); match != nil {
			lines[i] = match[1] + quoteYAMLString(envVars[match[2]]+match[3])
			continue
		}

		// An unquoted list item such as `- KEY={{env.X}}` is one scalar: splicing a
		// quoted value into its middle would leak the quotes into the variable.
		if match := composeEnvListItemInline.FindStringSubmatch(line); match != nil {
			raw := composeEnvPlaceholder.ReplaceAllStringFunc(match[2], func(m string) string {
				return envVars[composeEnvPlaceholder.FindStringSubmatch(m)[1]]
			})
			lines[i] = match[1] + quoteYAMLString(raw)
			continue
		}

		// A placeholder inside an already double-quoted value must not add its own
		// quotes, or the line becomes invalid YAML.
		inQuoted := composeQuotedEnvValue.MatchString(line)
		lines[i] = composeEnvPlaceholder.ReplaceAllStringFunc(line, func(match string) string {
			key := composeEnvPlaceholder.FindStringSubmatch(match)[1]
			quoted := quoteYAMLString(envVars[key])
			if inQuoted {
				return quoted[1 : len(quoted)-1]
			}
			return quoted
		})
	}
	return strings.Join(lines, "\n")
}

func renderGeneratedNetwork(b *strings.Builder, netCfg *NetworkConfig) {
	if netCfg == nil {
		return
	}

	mode := strings.TrimSpace(netCfg.Mode)
	switch mode {
	case "bridge", "host", "none":
		fmt.Fprintf(b, "    network_mode: %s\n", quoteYAMLString(mode))
	case "existing":
		networkName := strings.TrimSpace(netCfg.Name)
		if networkName == "" {
			return
		}
		networkKey := sanitizeServiceName(networkName)
		if networkKey == "" {
			networkKey = "app-network"
		}
		b.WriteString("    networks:\n")
		if len(netCfg.Aliases) == 0 && strings.TrimSpace(netCfg.IPv4Address) == "" && strings.TrimSpace(netCfg.IPv6Address) == "" && strings.TrimSpace(netCfg.MACAddress) == "" {
			fmt.Fprintf(b, "      %s: {}\n", networkKey)
		} else {
			fmt.Fprintf(b, "      %s:\n", networkKey)
			if len(netCfg.Aliases) > 0 {
				b.WriteString("        aliases:\n")
				for _, alias := range netCfg.Aliases {
					alias = strings.TrimSpace(alias)
					if alias != "" {
						fmt.Fprintf(b, "          - %s\n", quoteYAMLString(alias))
					}
				}
			}
			if value := strings.TrimSpace(netCfg.IPv4Address); value != "" {
				fmt.Fprintf(b, "        ipv4_address: %s\n", quoteYAMLString(value))
			}
			if value := strings.TrimSpace(netCfg.IPv6Address); value != "" {
				fmt.Fprintf(b, "        ipv6_address: %s\n", quoteYAMLString(value))
			}
			if value := strings.TrimSpace(netCfg.MACAddress); value != "" {
				fmt.Fprintf(b, "        mac_address: %s\n", quoteYAMLString(value))
			}
		}
		b.WriteString("networks:\n")
		fmt.Fprintf(b, "  %s:\n", networkKey)
		b.WriteString("    external: true\n")
		fmt.Fprintf(b, "    name: %s\n", quoteYAMLString(networkName))
	}
}

func renderComposeNetwork(netCfg *NetworkConfig) string {
	var b strings.Builder
	renderGeneratedNetwork(&b, netCfg)
	return strings.TrimRight(b.String(), "\n")
}

func renderComposeNetworkMode(netCfg *NetworkConfig) string {
	if netCfg == nil {
		return ""
	}
	mode := strings.TrimSpace(netCfg.Mode)
	if mode == "bridge" || mode == "host" || mode == "none" {
		return mode
	}
	return ""
}

func renderComposePorts(ports []PortMapping) string {
	if len(ports) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("    ports:\n")
	for _, p := range ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		value := fmt.Sprintf("%d:%d", p.Host, p.Container)
		if proto != "tcp" {
			value = fmt.Sprintf("%s/%s", value, proto)
		}
		fmt.Fprintf(&b, "      - %s\n", quoteYAMLString(value))
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderComposeVolumes(volumes []VolumeMount) string {
	if len(volumes) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("    volumes:\n")
	for _, v := range volumes {
		value := fmt.Sprintf("%s:%s", v.Host, v.Container)
		if v.ReadOnly {
			value += ":ro"
		}
		fmt.Fprintf(&b, "      - %s\n", quoteYAMLString(value))
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderComposeEnvironment(envVars map[string]string) string {
	if len(envVars) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("    environment:\n")
	for _, k := range sortedEnvKeys(envVars) {
		v := envVars[k]
		fmt.Fprintf(&b, "      - %s\n", yamlEnvItem(k, v))
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderComposeEnvironmentMap(envVars map[string]string) string {
	if len(envVars) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("    environment:\n")
	for _, k := range sortedEnvKeys(envVars) {
		fmt.Fprintf(&b, "      %s: %s\n", k, quoteYAMLString(envVars[k]))
	}
	return strings.TrimRight(b.String(), "\n")
}

func sortedEnvKeys(envVars map[string]string) []string {
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// yamlEnvItem renders one KEY=value environment entry. The entry stays bare
// unless YAML would read it as something other than a string: LiveKit ships
// LIVEKIT_KEYS=devkey: secret, and a bare "key: value" inside a sequence is a
// map, which is what made its generated compose unparseable. Quoting only the
// entries that need it keeps the ordinary ones readable in the stack editor.
func yamlEnvItem(key, value string) string {
	entry := key + "=" + value
	if envEntryNeedsQuoting(entry) {
		return quoteYAMLString(entry)
	}
	return entry
}

// envEntryNeedsQuoting reports whether a sequence entry would parse as
// something other than the string it reads as. A "key: value" pair or a
// trailing colon makes it a map, " #" starts a comment, surrounding whitespace
// is trimmed away, and the flow indicators open a collection.
func envEntryNeedsQuoting(entry string) bool {
	if entry == "" || strings.TrimSpace(entry) != entry {
		return true
	}
	if strings.Contains(entry, ": ") || strings.HasSuffix(entry, ":") {
		return true
	}
	if strings.Contains(entry, " #") {
		return true
	}
	return strings.ContainsAny(entry, "\"'\n\r\t{}[],")
}

func quoteYAMLString(value string) string {
	b, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(b)
}

// mergeOverrides applies user-provided overrides to the app defaults.
// If the user provides ports/volumes, those replace defaults.
// Env vars are merged: user values override defaults, but defaults fill gaps.
func mergeOverrides(app AppTemplate, req InstallRequest) ([]PortMapping, []VolumeMount, map[string]string) {
	ports := app.Ports
	if len(req.Ports) > 0 {
		ports = req.Ports
	}

	volumes := app.Volumes
	if len(req.Volumes) > 0 {
		volumes = req.Volumes
	}

	envVars := make(map[string]string)
	// Start with app defaults
	for _, ev := range app.EnvVars {
		if ev.Default != "" {
			envVars[ev.Key] = ev.Default
		}
	}
	// Override with user-provided values
	for k, v := range req.EnvVars {
		envVars[k] = v
	}

	return ports, volumes, envVars
}

// sanitizeServiceName converts a name to a valid docker compose service name.
func sanitizeServiceName(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			b.WriteRune(c)
		} else if c == ' ' {
			b.WriteRune('-')
		}
	}
	result := b.String()
	result = strings.Trim(result, "-_")
	if result == "" {
		result = "app"
	}
	return result
}

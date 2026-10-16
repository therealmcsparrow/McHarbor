// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/moby/moby/api/types/container"
	"gopkg.in/yaml.v3"

	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// Compose-only host-access reason codes.
const (
	// HostAccessOwnProject is a compose file whose project name is McHarbor's own, so
	// `up` would adopt and recreate McHarbor's container.
	HostAccessOwnProject = "mcharbor_project"
	// HostAccessComposeInclude is an include or extends-from-file the check cannot read,
	// so what it pulls in cannot be vouched for.
	HostAccessComposeInclude = "compose_include"
	// HostAccessUnresolved is a volume source still carrying an interpolation after every
	// known variable was substituted, so its real value is unknown.
	HostAccessUnresolved = "unresolved_source"
)

// ComposeHostAccess is one service's reasons, or Service "" for a file-level reason.
type ComposeHostAccess struct {
	Service string   `json:"service"`
	Reasons []string `json:"reasons"`
}

type composeCheckFile struct {
	Name     string                          `yaml:"name"`
	Include  yaml.Node                       `yaml:"include"`
	Services map[string]composeCheckSvc      `yaml:"services"`
	Volumes  map[string]*composeCheckTopVo   `yaml:"volumes"`
	Secrets  map[string]*composeCheckFileRef `yaml:"secrets"`
	Configs  map[string]*composeCheckFileRef `yaml:"configs"`
}

type composeCheckSvc struct {
	Privileged        yaml.Node   `yaml:"privileged"`
	Pid               string      `yaml:"pid"`
	CapAdd            []string    `yaml:"cap_add"`
	Devices           []yaml.Node `yaml:"devices"`
	DeviceCgroupRules []string    `yaml:"device_cgroup_rules"`
	SecurityOpt       []string    `yaml:"security_opt"`
	UsernsMode        string      `yaml:"userns_mode"`
	Ipc               string      `yaml:"ipc"`
	Uts               string      `yaml:"uts"`
	Cgroup            string      `yaml:"cgroup"`
	NetworkMode       string      `yaml:"network_mode"`
	Volumes           []yaml.Node `yaml:"volumes"`
	VolumesFrom       []string    `yaml:"volumes_from"`
	Extends           yaml.Node   `yaml:"extends"`
	Build             yaml.Node   `yaml:"build"`
	EnvFile           yaml.Node   `yaml:"env_file"`
	LabelFile         yaml.Node   `yaml:"label_file"`
	Secrets           []yaml.Node `yaml:"secrets"`
	Configs           []yaml.Node `yaml:"configs"`
}

type composeCheckTopVo struct {
	Name       string            `yaml:"name"`
	External   yaml.Node         `yaml:"external"`
	Driver     string            `yaml:"driver"`
	DriverOpts map[string]string `yaml:"driver_opts"`
}

// composeCheckFileRef is a top-level secret or config; a file source becomes a bind mount.
type composeCheckFileRef struct {
	File string `yaml:"file"`
}

// Compose-only reasons for paths the compose client itself reads, inside McHarbor's
// process: a build context and an env_file outside the project directory.
const (
	HostAccessBuildContext = "build_context"
	HostAccessEnvFile      = "env_file"
)

// ComposeHostAccessReasons reads a compose file the way `docker compose up` would in
// projectDir, interpolating from the process environment, projectDir/.env and env (in
// rising precedence), and lists every service that would reach McHarbor's host.
func ComposeHostAccessReasons(content []byte, projectDir string, env map[string]string, host SelfHost) ([]ComposeHostAccess, error) {
	return ComposeHostAccessReasonsWith(content, projectDir, env, host, nil)
}

// ComposeHostAccessReasonsWith is ComposeHostAccessReasons with lookup reading the named
// volumes that already exist on the daemon, since compose reuses one it did not create.
func ComposeHostAccessReasonsWith(content []byte, projectDir string, env map[string]string, host SelfHost, lookup VolumeLookup) ([]ComposeHostAccess, error) {
	return ComposeHostAccessReasonsProbe(content, projectDir, env, host, lookup, nil)
}

// ComposeHostAccessReasonsProbe is ComposeHostAccessReasonsWith that, when nothing else is
// found, runs probe once over every service's host bind sources: a service with a symlinked
// source gets host_bind_symlink, and an unanswered probe is a file-level unresolved_source.
func ComposeHostAccessReasonsProbe(content []byte, projectDir string, env map[string]string, host SelfHost, lookup VolumeLookup, probe BindPathProbe) ([]ComposeHostAccess, error) {
	out, file, project, err := composeHostAccess(content, projectDir, env, host, lookup)
	if err != nil || len(out) > 0 || probe == nil {
		return out, err
	}
	names := make([]string, 0, len(file.Services))
	for name := range file.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	perService := map[string][]string{}
	var all []string
	for _, name := range names {
		sources, ok := composeServiceBindSources(file.Services[name], file, project, projectDir, lookup)
		if !ok {
			return []ComposeHostAccess{{Reasons: []string{HostAccessUnresolved}}}, nil
		}
		perService[name] = sources
		all = appendUnique(all, sources...)
	}
	if len(all) == 0 {
		return nil, nil
	}
	linked, ok := runBindProbe(probe, all)
	if !ok {
		return []ComposeHostAccess{{Reasons: []string{HostAccessUnresolved}}}, nil
	}
	for _, name := range names {
		for _, src := range perService[name] {
			if linked[src] {
				out = append(out, ComposeHostAccess{Service: name, Reasons: []string{HostAccessBindSymlink}})
				break
			}
		}
	}
	return out, nil
}

func composeHostAccess(content []byte, projectDir string, env map[string]string, host SelfHost, lookup VolumeLookup) ([]ComposeHostAccess, composeCheckFile, string, error) {
	vars := composeVars(projectDir, env)
	var file composeCheckFile
	if err := yaml.Unmarshal([]byte(interpolate(string(content), vars)), &file); err != nil {
		return nil, file, "", fmt.Errorf("parsing compose file: %w", err)
	}
	project := strings.ToLower(strings.TrimSpace(file.Name))
	if project == "" {
		project = strings.ToLower(filepath.Base(filepath.Clean(projectDir)))
	}

	var out []ComposeHostAccess
	var fileReasons []string
	if host.ComposeProject != "" && project == host.ComposeProject {
		fileReasons = append(fileReasons, HostAccessOwnProject)
	}
	if file.Include.Kind != 0 {
		fileReasons = append(fileReasons, HostAccessComposeInclude)
	}
	if len(fileReasons) > 0 {
		out = append(out, ComposeHostAccess{Reasons: fileReasons})
	}

	names := make([]string, 0, len(file.Services))
	for name := range file.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		svc := file.Services[name]
		reasons := composeServiceReasons(svc, file, project, projectDir, host, lookup)
		if len(reasons) > 0 {
			out = append(out, ComposeHostAccess{Service: name, Reasons: reasons})
		}
	}
	return out, file, project, nil
}

func composeServiceReasons(svc composeCheckSvc, file composeCheckFile, project, projectDir string, host SelfHost, lookup VolumeLookup) []string {
	var out []string
	if JoinsNamespace(svc.Pid, svc.Ipc, svc.NetworkMode, svc.UsernsMode, svc.Uts, svc.Cgroup) {
		out = appendUnique(out, HostAccessJoinsNamespace)
	}
	if truthy(svc.Privileged) {
		out = appendUnique(out, HostAccessPrivileged)
	}
	if strings.EqualFold(strings.TrimSpace(svc.Pid), "host") {
		out = appendUnique(out, HostAccessPIDHost)
	}
	if CapAddIsSysAdmin(svc.CapAdd) {
		out = appendUnique(out, HostAccessSysAdmin)
	}
	out = appendUnique(out, CreateTimeReasons(&container.HostConfig{
		CapAdd:       svc.CapAdd,
		SecurityOpt:  svc.SecurityOpt,
		UsernsMode:   container.UsernsMode(strings.TrimSpace(svc.UsernsMode)),
		IpcMode:      container.IpcMode(strings.TrimSpace(svc.Ipc)),
		NetworkMode:  container.NetworkMode(strings.TrimSpace(svc.NetworkMode)),
		UTSMode:      container.UTSMode(strings.TrimSpace(svc.Uts)),
		CgroupnsMode: container.CgroupnsMode(strings.TrimSpace(svc.Cgroup)),
		Resources:    container.Resources{DeviceCgroupRules: svc.DeviceCgroupRules},
	})...)
	if len(svc.Devices) > 0 {
		out = appendUnique(out, HostAccessDevices)
	}
	if svc.Extends.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(svc.Extends.Content); i += 2 {
			if svc.Extends.Content[i].Value == "file" {
				out = appendUnique(out, HostAccessComposeInclude)
			}
		}
	}
	for _, ref := range svc.VolumesFrom {
		if VolumesFromIsMcHarbor(ref, host) {
			out = appendUnique(out, HostAccessVolumesFrom)
		}
	}
	for _, v := range svc.Volumes {
		kind, source := composeVolumeSource(v)
		if source == "" {
			continue
		}
		if strings.Contains(source, "$") {
			out = appendUnique(out, HostAccessUnresolved)
			continue
		}
		switch kind {
		case "bind":
			out = appendUnique(out, HostBindReasons(resolveComposePath(source, projectDir), host)...)
		case "volume":
			out = appendUnique(out, composeNamedVolumeReasons(source, file, project, host, lookup)...)
		}
	}
	out = appendUnique(out, composeFileRefReasons(svc.Secrets, file.Secrets, projectDir, host)...)
	out = appendUnique(out, composeFileRefReasons(svc.Configs, file.Configs, projectDir, host)...)
	for _, p := range append(composePathList(svc.EnvFile), composePathList(svc.LabelFile)...) {
		out = appendUnique(out, composeClientPathReasons(p, projectDir, HostAccessEnvFile, host)...)
	}
	for _, p := range composeBuildPaths(svc.Build) {
		out = appendUnique(out, composeClientPathReasons(p, projectDir, HostAccessBuildContext, host)...)
	}
	return out
}

// composeNamedVolumeReasons classifies a named volume: McHarbor's own, one the file declares
// with local bind driver_opts, or an existing one lookup finds carrying them.
func composeNamedVolumeReasons(key string, file composeCheckFile, project string, host SelfHost, lookup VolumeLookup) []string {
	name := composeVolumeName(key, file, project)
	out := HostVolumeReasons(name, host)
	if top := file.Volumes[key]; top != nil && len(top.DriverOpts) > 0 {
		for _, v := range top.DriverOpts {
			if strings.Contains(v, "$") {
				return appendUnique(out, HostAccessUnresolved)
			}
		}
		return appendUnique(out, VolumeDeviceReasons(top.Driver, top.DriverOpts, host, true)...)
	}
	return appendUnique(out, volumeLookupReasons(lookup, name, host, true)...)
}

// composeFileRefReasons classifies the secrets or configs a service uses whose top-level
// definition has a file source: compose bind-mounts that file into the container.
func composeFileRefReasons(refs []yaml.Node, defs map[string]*composeCheckFileRef, projectDir string, host SelfHost) []string {
	var out []string
	for _, src := range composeFileRefSources(refs, defs) {
		if strings.Contains(src, "$") {
			out = appendUnique(out, HostAccessUnresolved)
			continue
		}
		out = appendUnique(out, HostBindReasons(composeLocalPath(src, projectDir), host)...)
	}
	return out
}

// composeFileRefSources lists the file sources of the secrets or configs refs names.
func composeFileRefSources(refs []yaml.Node, defs map[string]*composeCheckFileRef) []string {
	var out []string
	for _, ref := range refs {
		name := ref.Value
		if ref.Kind == yaml.MappingNode {
			name = ""
			for i := 0; i+1 < len(ref.Content); i += 2 {
				if ref.Content[i].Value == "source" {
					name = ref.Content[i+1].Value
				}
			}
		}
		def := defs[strings.TrimSpace(name)]
		if def == nil || strings.TrimSpace(def.File) == "" {
			continue
		}
		out = append(out, strings.TrimSpace(def.File))
	}
	return out
}

// composeServiceBindSources lists every host path a service would bind once the file passed
// the denylist: bind volumes, file secrets and configs, and named volumes with bind options
// (declared in the file, or already on the daemon). ok is false when an existing volume
// cannot be read, and the caller fails closed.
func composeServiceBindSources(svc composeCheckSvc, file composeCheckFile, project, projectDir string, lookup VolumeLookup) ([]string, bool) {
	var out, names []string
	for _, v := range svc.Volumes {
		kind, source := composeVolumeSource(v)
		switch {
		case source == "":
		case kind == "bind":
			out = append(out, resolveComposePath(source, projectDir))
		case kind == "volume":
			if top := file.Volumes[source]; top != nil && len(top.DriverOpts) > 0 {
				if dev, ok := bindDeviceOf(top.Driver, top.DriverOpts); ok {
					out = append(out, dev)
				}
			} else {
				names = appendUnique(names, composeVolumeName(source, file, project))
			}
		}
	}
	for _, src := range append(composeFileRefSources(svc.Secrets, file.Secrets), composeFileRefSources(svc.Configs, file.Configs)...) {
		out = append(out, composeLocalPath(src, projectDir))
	}
	devs, err := existingBindDevices(lookup, names)
	if err != nil {
		return nil, false
	}
	return cleanBindSources(append(out, devs...)), true
}

// composeClientPathReasons classifies a path the compose client reads itself (an env_file,
// a build context): it runs in McHarbor's process, so a path outside the project directory
// reads McHarbor's own files, and reason is reported with any sensitive-path reasons.
func composeClientPathReasons(p, projectDir, reason string, host SelfHost) []string {
	if strings.Contains(p, "$") {
		return []string{HostAccessUnresolved}
	}
	resolved := composeLocalPath(p, projectDir)
	dir := path.Clean(filepath.ToSlash(projectDir))
	if resolved == dir || isPathAncestor(dir, resolved) {
		return nil
	}
	return appendUnique([]string{reason}, HostPathReasons(resolved, host)...)
}

// composeLocalPath resolves a compose path the way the compose client does: "~" against its
// home, anything else relative against the project directory.
func composeLocalPath(p, projectDir string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") {
		return resolveComposePath(p, projectDir)
	}
	return path.Join(filepath.ToSlash(projectDir), p)
}

// composePathList reads an env_file or label_file entry: a string, a list of strings, or a
// list of {path, required} mappings.
func composePathList(n yaml.Node) []string {
	var out []string
	switch n.Kind {
	case yaml.ScalarNode:
		if strings.TrimSpace(n.Value) != "" {
			out = append(out, n.Value)
		}
	case yaml.SequenceNode:
		for _, item := range n.Content {
			switch item.Kind {
			case yaml.ScalarNode:
				out = append(out, item.Value)
			case yaml.MappingNode:
				for i := 0; i+1 < len(item.Content); i += 2 {
					if item.Content[i].Value == "path" {
						out = append(out, item.Content[i+1].Value)
					}
				}
			}
		}
	}
	return out
}

// composeBuildPaths lists the local paths a build section sends to the daemon: the context,
// a Dockerfile given by path, and additional contexts. Remote contexts are not host paths.
func composeBuildPaths(n yaml.Node) []string {
	var out []string
	add := func(p string, base string) {
		p = strings.TrimSpace(p)
		if p == "" || strings.Contains(p, "://") || strings.HasPrefix(p, "git@") ||
			strings.HasPrefix(p, "service:") || strings.HasPrefix(p, "target:") {
			return
		}
		if base != "" && !strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "~") {
			p = path.Join(base, p)
		}
		out = append(out, p)
	}
	switch n.Kind {
	case yaml.ScalarNode:
		add(n.Value, "")
	case yaml.MappingNode:
		context, dockerfile := ".", ""
		var extra []string
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, val := n.Content[i].Value, n.Content[i+1]
			switch key {
			case "context":
				context = val.Value
			case "dockerfile":
				dockerfile = val.Value
			case "additional_contexts":
				switch val.Kind {
				case yaml.MappingNode:
					for j := 1; j < len(val.Content); j += 2 {
						extra = append(extra, val.Content[j].Value)
					}
				case yaml.SequenceNode:
					for _, item := range val.Content {
						if _, v, ok := strings.Cut(item.Value, "="); ok {
							extra = append(extra, v)
						}
					}
				}
			}
		}
		add(context, "")
		if dockerfile != "" && !strings.Contains(context, "://") {
			add(dockerfile, context)
		}
		for _, e := range extra {
			add(e, "")
		}
	}
	return out
}

// composeVolumeSource returns ("bind"|"volume", source) for one service volume entry in
// short ("src:dst[:mode]") or long ({type, source}) form; an anonymous volume has no source.
func composeVolumeSource(v yaml.Node) (string, string) {
	switch v.Kind {
	case yaml.ScalarNode:
		parts := strings.SplitN(v.Value, ":", 2)
		if len(parts) < 2 {
			return "", ""
		}
		src := strings.TrimSpace(parts[0])
		if isComposePathSource(src) {
			return "bind", src
		}
		return "volume", src
	case yaml.MappingNode:
		var typ, src string
		for i := 0; i+1 < len(v.Content); i += 2 {
			switch v.Content[i].Value {
			case "type":
				typ = strings.TrimSpace(v.Content[i+1].Value)
			case "source":
				src = strings.TrimSpace(v.Content[i+1].Value)
			}
		}
		switch typ {
		case "bind":
			return "bind", src
		case "volume", "":
			if isComposePathSource(src) {
				return "bind", src
			}
			return "volume", src
		}
	}
	return "", ""
}

func isComposePathSource(src string) bool {
	return strings.HasPrefix(src, "/") || strings.HasPrefix(src, ".") || strings.HasPrefix(src, "~")
}

// resolveComposePath makes a relative bind absolute the way compose does, against the
// project directory; "~" resolves to the compose process's home.
func resolveComposePath(src, projectDir string) string {
	switch {
	case strings.HasPrefix(src, "~"):
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			home = "/root"
		}
		return path.Join(filepath.ToSlash(home), strings.TrimPrefix(src, "~"))
	case strings.HasPrefix(src, "."):
		return path.Join(filepath.ToSlash(projectDir), src)
	}
	return path.Clean(src)
}

// composeVolumeName is the daemon-side name compose gives a named volume: its explicit
// name, its key when external, otherwise "<project>_<key>".
func composeVolumeName(key string, file composeCheckFile, project string) string {
	top, ok := file.Volumes[key]
	if !ok {
		return key
	}
	if top != nil && strings.TrimSpace(top.Name) != "" {
		return strings.TrimSpace(top.Name)
	}
	if top != nil && (truthy(top.External) || top.External.Kind == yaml.MappingNode) {
		return key
	}
	return project + "_" + key
}

func truthy(n yaml.Node) bool {
	if n.Kind != yaml.ScalarNode {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(n.Value)) {
	case "true", "yes", "on", "1", "y":
		return true
	}
	return false
}

func composeVars(projectDir string, env map[string]string) map[string]string {
	vars := map[string]string{}
	for _, entry := range os.Environ() {
		if k, v, ok := strings.Cut(entry, "="); ok {
			vars[k] = v
		}
	}
	if projectDir != "" {
		if f, err := os.Open(filepath.Join(projectDir, ".env")); err == nil {
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				line = strings.TrimPrefix(line, "export ")
				if k, v, ok := strings.Cut(line, "="); ok {
					vars[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
				}
			}
			closeutil.Report(f.Close(), ".env")
		}
	}
	for k, v := range env {
		vars[k] = v
	}
	return vars
}

var interpolationRe = regexp.MustCompile(`\$\$|\$\{([A-Za-z_][A-Za-z0-9_]*)(?:(:?[-?+])([^}]*))?\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// interpolate applies compose's ${VAR}, ${VAR:-default}, ${VAR-default}, ${VAR:+alt} and
// $VAR substitution; a form it cannot resolve is left in place and reported unresolved.
func interpolate(s string, vars map[string]string) string {
	return interpolationRe.ReplaceAllStringFunc(s, func(m string) string {
		if m == "$$" {
			return "$"
		}
		sub := interpolationRe.FindStringSubmatch(m)
		name, op, arg := sub[1], sub[2], sub[3]
		if name == "" {
			name = sub[4]
		}
		val, set := vars[name]
		switch op {
		case ":-":
			if val == "" {
				return arg
			}
		case "-":
			if !set {
				return arg
			}
		case ":+":
			if val != "" {
				return arg
			}
			return ""
		case "+":
			if set {
				return arg
			}
			return ""
		case ":?", "?":
			if !set || (op == ":?" && val == "") {
				return m
			}
		}
		return val
	})
}

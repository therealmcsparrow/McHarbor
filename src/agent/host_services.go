// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// hostServiceUnitPattern is the only shape of unit name the agent will act on.
// It admits no path separator, so a name can never climb out of the drop-in
// directory it is joined onto.
var hostServiceUnitPattern = regexp.MustCompile(`^[A-Za-z0-9@._:-]+\.service$`)

const (
	// hostServiceUnitMaxLen bounds a unit name well above systemd's own limit.
	hostServiceUnitMaxLen = 256
	// hostServiceOverrideMaxBytes bounds a drop-in; a real one is a few lines.
	hostServiceOverrideMaxBytes = 64 << 10
	// hostServiceQueryTimeout bounds a read-only systemctl call.
	hostServiceQueryTimeout = 30 * time.Second
	// hostServiceActionTimeout bounds start/stop/restart, which block until the
	// unit's own start or stop timeout — 90s by systemd default.
	hostServiceActionTimeout = 100 * time.Second
	// hostServiceShowChunk is how many units one `systemctl show` call covers.
	hostServiceShowChunk = 200
)

// Host service error codes carried in HostServicePayload.ErrorCode.
const (
	hostServiceErrInvalidUnit    = "invalid_unit"
	hostServiceErrInvalidAction  = "invalid_action"
	hostServiceErrInvalidContent = "invalid_content"
	hostServiceErrProtected      = "protected"
	hostServiceErrUnsupported    = "unsupported"
	hostServiceErrFailed         = "failed"
)

// errHostServiceInvalidUnit is returned for any name the pattern refuses.
var errHostServiceInvalidUnit = errors.New("invalid unit name")

// validateHostServiceUnit refuses anything but a plain .service unit name. A
// leading dash would reach systemctl as an option, and a leading dot names a
// hidden drop-in directory nobody meant to create.
func validateHostServiceUnit(name string) error {
	if name == "" || len(name) > hostServiceUnitMaxLen {
		return errHostServiceInvalidUnit
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return errHostServiceInvalidUnit
	}
	if strings.HasPrefix(name, "-") || strings.HasPrefix(name, ".") {
		return errHostServiceInvalidUnit
	}
	if !hostServiceUnitPattern.MatchString(name) {
		return errHostServiceInvalidUnit
	}
	return nil
}

// hostServiceProtection names why a unit may not be stopped or disabled, or ""
// when it may. The list must match modules/hostservices/guard.go on the server:
// both sides refuse, so a patched server alone cannot cut a host off.
func hostServiceProtection(unit string) string {
	switch {
	case strings.HasPrefix(unit, "mcharbor-agent"):
		return "the McHarbor agent itself"
	case unit == "ssh.service" || unit == "sshd.service":
		return "remote shell access to the host"
	case strings.HasPrefix(unit, "systemd-"):
		return "a core systemd component"
	case strings.HasPrefix(unit, "dbus"):
		return "the system message bus"
	case unit == "docker.service" || unit == "containerd.service":
		return "the container runtime the agent drives"
	}
	return ""
}

// hostServiceIsAgentUnit reports whether unit is this agent's own service,
// whose restart drops the connection the request came in on.
func hostServiceIsAgentUnit(unit string) bool {
	return strings.HasPrefix(unit, "mcharbor-agent")
}

// hostServiceActionAllowed refuses stop, disable and mask on a protected unit.
// Restart stays allowed: it brings the unit straight back.
func hostServiceActionAllowed(unit, action string) error {
	switch action {
	case HostServiceActionStart, HostServiceActionRestart, HostServiceActionEnable:
		return nil
	case HostServiceActionStop, HostServiceActionDisable, "mask":
		if reason := hostServiceProtection(unit); reason != "" {
			return fmt.Errorf("%s is protected (%s) and cannot be %s from McHarbor", unit, reason, pastParticiple(action))
		}
		return nil
	}
	return fmt.Errorf("unknown action %q", action)
}

func validHostServiceAction(action string) bool {
	switch action {
	case HostServiceActionStart, HostServiceActionStop, HostServiceActionRestart,
		HostServiceActionEnable, HostServiceActionDisable:
		return true
	}
	return false
}

func pastParticiple(action string) string {
	switch action {
	case HostServiceActionStop:
		return "stopped"
	case HostServiceActionDisable:
		return "disabled"
	case "mask":
		return "masked"
	}
	return action + "ed"
}

// hostCommandRunner runs one host command and returns its combined output,
// capped, and whether the cap was hit. Tests replace it; production shells out.
type hostCommandRunner func(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, bool, error)

type listUnitJSON struct {
	Unit        string `json:"unit"`
	Load        string `json:"load"`
	Active      string `json:"active"`
	Sub         string `json:"sub"`
	Description string `json:"description"`
}

type unitFileJSON struct {
	UnitFile string `json:"unit_file"`
	State    string `json:"state"`
}

// parseListUnitsJSON reads `systemctl list-units --output=json`. An older
// systemctl ignores --output for list-units and prints a table, which fails
// here and sends the caller to the plain parser.
func parseListUnitsJSON(data []byte) ([]HostServiceUnit, error) {
	var rows []listUnitJSON
	if err := json.Unmarshal(bytes.TrimSpace(data), &rows); err != nil {
		return nil, err
	}
	units := make([]HostServiceUnit, 0, len(rows))
	for _, row := range rows {
		if !strings.HasSuffix(row.Unit, ".service") {
			continue
		}
		units = append(units, HostServiceUnit{
			Name: row.Unit, LoadState: row.Load, ActiveState: row.Active,
			SubState: row.Sub, Description: row.Description,
		})
	}
	return units, nil
}

// parseListUnitsPlain reads `systemctl list-units --plain --no-legend`. Some
// versions still print the failed-unit bullet despite --plain, so it is skipped.
func parseListUnitsPlain(data []byte) []HostServiceUnit {
	var units []HostServiceUnit
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "●"), "*"))
		fields := strings.Fields(line)
		if len(fields) < 4 || !strings.HasSuffix(fields[0], ".service") {
			continue
		}
		units = append(units, HostServiceUnit{
			Name: fields[0], LoadState: fields[1], ActiveState: fields[2], SubState: fields[3],
			Description: strings.Join(fields[4:], " "),
		})
	}
	return units
}

// parseUnitFilesJSON reads `systemctl list-unit-files --output=json`.
func parseUnitFilesJSON(data []byte) (map[string]string, error) {
	var rows []unitFileJSON
	if err := json.Unmarshal(bytes.TrimSpace(data), &rows); err != nil {
		return nil, err
	}
	files := make(map[string]string, len(rows))
	for _, row := range rows {
		name := filepath.Base(row.UnitFile)
		if strings.HasSuffix(name, ".service") {
			files[name] = row.State
		}
	}
	return files, nil
}

// parseUnitFilesPlain reads `systemctl list-unit-files --plain --no-legend`,
// whose columns are UNIT FILE, STATE and, on newer versions, VENDOR PRESET.
func parseUnitFilesPlain(data []byte) map[string]string {
	files := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || !strings.HasSuffix(fields[0], ".service") {
			continue
		}
		files[fields[0]] = fields[1]
	}
	return files
}

// parseShowBlocks reads `systemctl show -p … unit…`: one Key=Value block per
// unit, separated by blank lines, keyed here by the block's Id.
func parseShowBlocks(data []byte) map[string]map[string]string {
	out := map[string]map[string]string{}
	current := map[string]string{}
	flush := func() {
		if id := current["Id"]; id != "" {
			out[id] = current
		}
		current = map[string]string{}
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			current[key] = value
		}
	}
	flush()
	return out
}

// hostServiceFailureReason turns systemd's result properties into the one
// sentence the Services tab shows behind a failed unit's error icon.
func hostServiceFailureReason(props map[string]string) string {
	var parts []string
	if result := props["Result"]; result != "" && result != "success" {
		parts = append(parts, "result: "+result)
	}
	code, status := props["ExecMainCode"], props["ExecMainStatus"]
	if status != "" && status != "0" {
		switch code {
		case "2":
			parts = append(parts, "main process killed by signal "+status)
		default:
			parts = append(parts, "main process exited with status "+status)
		}
	}
	if text := strings.TrimSpace(props["StatusText"]); text != "" {
		parts = append(parts, "status: "+text)
	}
	return strings.Join(parts, "; ")
}

// mergeHostServices joins the loaded units, the installed unit files and the
// show properties into one sorted list. A unit file that is not loaded still
// appears, inactive, because "installed but never started" is worth seeing.
func mergeHostServices(units []HostServiceUnit, files map[string]string, show map[string]map[string]string) []HostServiceUnit {
	byName := make(map[string]*HostServiceUnit, len(units)+len(files))
	for i := range units {
		unit := units[i]
		byName[unit.Name] = &unit
	}
	for name, state := range files {
		if unit, ok := byName[name]; ok {
			unit.UnitFileState = state
			continue
		}
		byName[name] = &HostServiceUnit{Name: name, UnitFileState: state, ActiveState: "inactive", SubState: "dead"}
	}
	merged := make([]HostServiceUnit, 0, len(byName))
	for _, unit := range byName {
		unit.Template = strings.HasSuffix(unit.Name, "@.service")
		if props, ok := show[unit.Name]; ok {
			if pid, err := strconv.Atoi(props["MainPID"]); err == nil && pid > 0 {
				unit.MainPID = pid
			}
			if unit.ActiveState == "failed" {
				unit.Result = props["Result"]
				unit.FailureReason = hostServiceFailureReason(props)
			}
		}
		if unit.ActiveState == "failed" && unit.FailureReason == "" {
			unit.FailureReason = "the unit is in the failed state"
		}
		merged = append(merged, *unit)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].Name < merged[j].Name })
	return merged
}

// listHostServices asks systemctl for the machine-readable listing first and
// falls back to the plain table when the host's systemd predates JSON output.
func listHostServices(ctx context.Context, run hostCommandRunner) ([]HostServiceUnit, bool, error) {
	var truncated bool
	out, cut, err := run(ctx, hostServiceQueryTimeout, "systemctl", "list-units", "--type=service", "--all", "--output=json", "--no-pager")
	units, parseErr := parseListUnitsJSON(out)
	if err != nil || parseErr != nil || cut {
		out, cut, err = run(ctx, hostServiceQueryTimeout, "systemctl", "list-units", "--type=service", "--all", "--plain", "--no-legend", "--no-pager")
		if err != nil {
			return nil, false, fmt.Errorf("systemctl list-units: %w: %s", err, firstLines(out))
		}
		units = parseListUnitsPlain(out)
	}
	truncated = truncated || cut

	files := map[string]string{}
	out, cut, err = run(ctx, hostServiceQueryTimeout, "systemctl", "list-unit-files", "--type=service", "--output=json", "--no-pager")
	if parsed, parseErr := parseUnitFilesJSON(out); err == nil && parseErr == nil && !cut {
		files = parsed
	} else {
		out, cut, err = run(ctx, hostServiceQueryTimeout, "systemctl", "list-unit-files", "--type=service", "--plain", "--no-legend", "--no-pager")
		if err == nil {
			files = parseUnitFilesPlain(out)
		}
		truncated = truncated || cut
	}

	show := map[string]map[string]string{}
	var loaded []string
	for _, unit := range units {
		if unit.LoadState == "loaded" {
			loaded = append(loaded, unit.Name)
		}
	}
	for start := 0; start < len(loaded); start += hostServiceShowChunk {
		end := min(start+hostServiceShowChunk, len(loaded))
		args := []string{"show", "--no-pager", "-p", "Id", "-p", "MainPID", "-p", "Result", "-p", "StatusText",
			"-p", "ExecMainStatus", "-p", "ExecMainCode", "--"}
		args = append(args, loaded[start:end]...)
		out, _, err := run(ctx, hostServiceQueryTimeout, "systemctl", args...)
		if err != nil {
			continue
		}
		for id, props := range parseShowBlocks(out) {
			show[id] = props
		}
	}
	return mergeHostServices(units, files, show), truncated, nil
}

// hostServiceDropInDir is the directory `systemctl edit` would use for unit.
func hostServiceDropInDir(base, unit string) string {
	return filepath.Join(base, unit+".d")
}

// validateOverrideContent refuses a drop-in systemd could not read back or that
// no operator could have meant: oversized, binary, or not UTF-8.
func validateOverrideContent(content string) error {
	if len(content) > hostServiceOverrideMaxBytes {
		return fmt.Errorf("the override is larger than %d bytes", hostServiceOverrideMaxBytes)
	}
	if !utf8.ValidString(content) || strings.ContainsRune(content, 0) {
		return errors.New("the override is not UTF-8 text")
	}
	return nil
}

// writeHostServiceOverride writes base/<unit>.d/override.conf atomically: a
// temp file in the same directory, synced, 0644, handed to root, then renamed,
// so systemd never reads a half-written drop-in.
func writeHostServiceOverride(base, unit, content string, chown func(string) error) (string, error) {
	if err := validateHostServiceUnit(unit); err != nil {
		return "", err
	}
	if err := validateOverrideContent(content); err != nil {
		return "", err
	}
	dir := hostServiceDropInDir(base, unit)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}
	target := filepath.Join(dir, "override.conf")
	tmp, err := os.CreateTemp(dir, ".override.conf.*")
	if err != nil {
		return "", fmt.Errorf("creating a temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			if rmErr := os.Remove(tmpName); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
				fmt.Fprintf(os.Stderr, "host services: removing %s: %v\n", tmpName, rmErr)
			}
		}
	}()
	if _, err := tmp.WriteString(content); err != nil {
		errlog.Close(tmp)
		return "", fmt.Errorf("writing %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		errlog.Close(tmp)
		return "", fmt.Errorf("syncing %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("closing %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return "", fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if chown != nil {
		if err := chown(tmpName); err != nil {
			return "", fmt.Errorf("chown %s: %w", tmpName, err)
		}
	}
	if err := os.Rename(tmpName, target); err != nil {
		return "", fmt.Errorf("renaming into %s: %w", target, err)
	}
	committed = true
	return target, nil
}

// removeHostServiceOverride deletes base/<unit>.d/override.conf and then the
// directory, but only when nothing else is left in it: other drop-ins in the
// same directory belong to someone else. A missing override is not an error.
func removeHostServiceOverride(base, unit string) (bool, error) {
	if err := validateHostServiceUnit(unit); err != nil {
		return false, err
	}
	dir := hostServiceDropInDir(base, unit)
	target := filepath.Join(dir, "override.conf")
	removed := true
	if err := os.Remove(target); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return false, fmt.Errorf("removing %s: %w", target, err)
		}
		removed = false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return removed, nil
		}
		return removed, fmt.Errorf("reading %s: %w", dir, err)
	}
	if len(entries) == 0 {
		if err := os.Remove(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return removed, fmt.Errorf("removing %s: %w", dir, err)
		}
	}
	return removed, nil
}

// readHostServiceOverride returns the current override and whether it exists.
func readHostServiceOverride(base, unit string) (string, bool, error) {
	target := filepath.Join(hostServiceDropInDir(base, unit), "override.conf")
	info, err := os.Stat(target)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	if info.Size() > hostServiceOverrideMaxBytes {
		return "", true, fmt.Errorf("%s is larger than %d bytes", target, hostServiceOverrideMaxBytes)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", false, err
	}
	return string(data), true, nil
}

// firstLines trims command output to what fits in an error the operator reads.
func firstLines(out []byte) string {
	text := strings.TrimSpace(string(out))
	if len(text) > 2000 {
		text = text[:2000] + "…"
	}
	return text
}

// hostServiceExecutor runs the operations; base and chown are fields so tests
// point it at a temp directory instead of /etc/systemd/system.
type hostServiceExecutor struct {
	run   hostCommandRunner
	base  string
	chown func(string) error
}

// execute performs one request and fills the reply. It never panics on bad
// input: every refusal comes back as ErrorCode plus a sentence.
func (e *hostServiceExecutor) execute(ctx context.Context, req HostServicePayload) *HostServicePayload {
	reply := &HostServicePayload{Op: req.Op, Unit: req.Unit, Action: req.Action}
	fail := func(code string, err error) *HostServicePayload {
		reply.Success = false
		reply.ErrorCode = code
		reply.Error = err.Error()
		return reply
	}
	if req.Op != HostServiceOpList && req.Op != HostServiceOpDaemonReload {
		if err := validateHostServiceUnit(req.Unit); err != nil {
			return fail(hostServiceErrInvalidUnit, fmt.Errorf("%q is not a valid .service unit name", req.Unit))
		}
	}

	switch req.Op {
	case HostServiceOpList:
		units, truncated, err := listHostServices(ctx, e.run)
		if err != nil {
			return fail(hostServiceErrFailed, err)
		}
		reply.Units = units
		reply.Truncated = truncated

	case HostServiceOpAction:
		if !validHostServiceAction(req.Action) {
			return fail(hostServiceErrInvalidAction, fmt.Errorf("unknown action %q", req.Action))
		}
		if err := hostServiceActionAllowed(req.Unit, req.Action); err != nil {
			return fail(hostServiceErrProtected, err)
		}
		if req.Action == HostServiceActionRestart && hostServiceIsAgentUnit(req.Unit) {
			reply.Warning = HostServiceWarningAgentRestart
			break
		}
		out, _, err := e.run(ctx, hostServiceActionTimeout, "systemctl", req.Action, "--", req.Unit)
		if err != nil {
			return fail(hostServiceErrFailed, fmt.Errorf("systemctl %s %s failed: %s", req.Action, req.Unit, orError(out, err)))
		}

	case HostServiceOpOverrideGet:
		out, truncated, err := e.run(ctx, hostServiceQueryTimeout, "systemctl", "cat", "--no-pager", "--", req.Unit)
		if err != nil {
			return fail(hostServiceErrFailed, fmt.Errorf("systemctl cat %s failed: %s", req.Unit, orError(out, err)))
		}
		reply.Effective = string(out)
		reply.Truncated = truncated
		content, exists, err := readHostServiceOverride(e.base, req.Unit)
		if err != nil {
			return fail(hostServiceErrFailed, err)
		}
		reply.Override = content
		reply.OverrideExists = exists
		reply.OverridePath = filepath.ToSlash(filepath.Join(hostServiceDropInDir(e.base, req.Unit), "override.conf"))

	case HostServiceOpOverridePut:
		if err := validateOverrideContent(req.Content); err != nil {
			return fail(hostServiceErrInvalidContent, err)
		}
		path, err := writeHostServiceOverride(e.base, req.Unit, req.Content, e.chown)
		if err != nil {
			return fail(hostServiceErrFailed, err)
		}
		reply.OverridePath = filepath.ToSlash(path)
		reply.OverrideExists = true
		if req.Reload {
			if out, _, err := e.run(ctx, hostServiceQueryTimeout, "systemctl", "daemon-reload"); err != nil {
				return fail(hostServiceErrFailed, fmt.Errorf("the override was saved but daemon-reload failed: %s", orError(out, err)))
			}
			reply.Reloaded = true
		}
		if req.Verify {
			reply.VerifyOutput = e.verify(ctx, req.Unit)
		}

	case HostServiceOpOverrideDel:
		if _, err := removeHostServiceOverride(e.base, req.Unit); err != nil {
			return fail(hostServiceErrFailed, err)
		}
		if out, _, err := e.run(ctx, hostServiceQueryTimeout, "systemctl", "daemon-reload"); err != nil {
			return fail(hostServiceErrFailed, fmt.Errorf("the override was removed but daemon-reload failed: %s", orError(out, err)))
		}
		reply.Reloaded = true

	case HostServiceOpDaemonReload:
		if out, _, err := e.run(ctx, hostServiceQueryTimeout, "systemctl", "daemon-reload"); err != nil {
			return fail(hostServiceErrFailed, fmt.Errorf("systemctl daemon-reload failed: %s", orError(out, err)))
		}
		reply.Reloaded = true

	default:
		return fail(hostServiceErrInvalidAction, fmt.Errorf("unknown host service operation %q", req.Op))
	}
	reply.Success = true
	return reply
}

// verify runs systemd-analyze on the unit's fragment. Its findings are a
// warning, never a refusal: verify flags harmless things on many stock units.
func (e *hostServiceExecutor) verify(ctx context.Context, unit string) string {
	out, _, err := e.run(ctx, hostServiceQueryTimeout, "systemctl", "show", "-p", "FragmentPath", "--value", "--", unit)
	fragment := strings.TrimSpace(string(out))
	if err != nil || fragment == "" {
		return ""
	}
	out, _, err = e.run(ctx, hostServiceQueryTimeout, "systemd-analyze", "verify", "--", fragment)
	if err != nil && len(out) == 0 {
		return err.Error()
	}
	return strings.TrimSpace(string(out))
}

func orError(out []byte, err error) string {
	if text := firstLines(out); text != "" {
		return text
	}
	return err.Error()
}

// handleHostServiceRequest answers one host_service_request. It runs on its own
// goroutine, because a stop can block for the unit's full stop timeout.
//
// Restarting the agent's own unit is answered first and performed after: the
// restart kills this process, so a reply sent afterwards would never leave.
func (a *Agent) handleHostServiceRequest(ctx context.Context, conn *websocket.Conn, reqID string, req HostServicePayload) {
	var reply *HostServicePayload
	executor, err := newHostServiceExecutor()
	if err != nil {
		reply = &HostServicePayload{Op: req.Op, Unit: req.Unit, Action: req.Action, ErrorCode: hostServiceErrUnsupported, Error: err.Error()}
	} else {
		reply = executor.execute(ctx, req)
	}

	writeMu.Lock()
	writeErr := conn.WriteJSON(WSMessage{Type: MsgHostServiceResponse, ID: reqID, HostService: reply})
	writeMu.Unlock()
	if writeErr != nil {
		a.logger.Warn("host services: sending the reply failed", "op", req.Op, "error", writeErr)
	}

	if reply.Success && reply.Warning == HostServiceWarningAgentRestart && executor != nil {
		a.logger.Info("host services: restarting the agent's own unit on request", "unit", req.Unit)
		// --no-block queues the job and returns, so systemd, not this process,
		// sees the restart through; the pause lets the reply above flush.
		time.Sleep(time.Second)
		restartCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), hostServiceQueryTimeout)
		defer cancel()
		if out, _, err := executor.run(restartCtx, hostServiceQueryTimeout, "systemctl", "--no-block", "restart", "--", req.Unit); err != nil {
			a.logger.Error("host services: restarting the agent unit failed", "unit", req.Unit, "error", err, "output", firstLines(out))
		}
	}
}

// agentCapabilities lists the optional features this agent can serve on this
// host, for AuthPayload.Capabilities.
func agentCapabilities() []string {
	var caps []string
	if hostServicesCapable() {
		caps = append(caps, HostServiceCapability)
	}
	return caps
}

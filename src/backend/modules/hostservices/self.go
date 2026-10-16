// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package hostservices

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/selfhost"
)

// selfRunner runs one `chroot /host …` script on the machine McHarbor is
// installed on; tests substitute a scripted host for the utility container.
type selfRunner func(ctx context.Context, script string, readWrite bool, timeout time.Duration) (*selfhost.Result, error)

// Self-host budgets, each a little shorter than the matching server bound so
// the host's own answer wins the race against call()'s deadline.
const (
	selfQueryTimeout    = 55 * time.Second
	selfActionTimeout   = 105 * time.Second
	selfOverrideTimeout = 85 * time.Second
	selfReloadTimeout   = 40 * time.Second
	// selfMarker prefixes every line the scripts emit for parsing; anything
	// else on the stream (a login banner, a TTY echo) is ignored.
	selfMarker = "@@MCHARBOR@@ "
	// systemdEtcDir is where administrator drop-ins live on the host.
	systemdEtcDir = "/etc/systemd/system"
)

// selfPrelude sets up the helpers every script uses and answers nosystemd when
// the host is not running systemd, so each op can tell that apart from a failure.
const selfPrelude = `export SYSTEMD_COLORS=0 SYSTEMD_PAGER= SYSTEMD_LESS= LC_ALL=C COLUMNS=512
emit() { printf "@@MCHARBOR@@ %s %s\n" "$1" "$2"; }
b64() { base64 | tr -d "\n\r"; }
if [ ! -d /run/systemd/system ] || ! command -v systemctl >/dev/null 2>&1; then emit nosystemd 1; exit 0; fi
`

// selfLink performs host service operations on McHarbor's own machine through
// the system module's self-host runner. It answers in the agent's reply shape,
// so call(), the refusal mapping and the handlers are shared with agent hosts.
type selfLink struct {
	run selfRunner
	// unreachable extracts the operator-facing reason from a runner error
	// when the host could not be reached at all.
	unreachable func(error) (string, bool)
}

// newSelfLink binds the link to the system module's self-host runner.
func newSelfLink(sys selfhost.Runner) *selfLink {
	return &selfLink{run: sys.RunSelfHost, unreachable: sys.SelfHostUnreachableReason}
}

// selfCapture runs cmd and emits its exit code and base64 output under name.
func selfCapture(name, cmd string) string {
	return fmt.Sprintf("out=\"$(%s 2>&1)\"; rc=$?; emit %s_rc \"$rc\"; emit %s \"$(printf %%s \"$out\" | b64)\"\n", cmd, name, name)
}

// selfListScript collects every source list needs in one run, since each run
// starts a utility container; the JSON and plain forms both come back so an
// old systemctl that ignores --output still parses.
func selfListScript() string {
	var b strings.Builder
	b.WriteString(selfPrelude)
	b.WriteString(selfCapture("units_json", "systemctl list-units --type=service --all --output=json --no-pager"))
	b.WriteString(selfCapture("units_plain", "systemctl list-units --type=service --all --plain --no-legend --no-pager --full"))
	b.WriteString(selfCapture("files_json", "systemctl list-unit-files --type=service --output=json --no-pager"))
	b.WriteString(selfCapture("files_plain", "systemctl list-unit-files --type=service --plain --no-legend --no-pager --full"))
	b.WriteString(selfCapture("show", `systemctl show --no-pager -p Id -p MainPID -p Result -p StatusText -p ExecMainStatus -p ExecMainCode -- "*.service"`))
	return b.String()
}

// selfDropInDir is the directory `systemctl edit` would use for unit. unit has
// passed ValidateUnit, so it carries no quote, space or path separator.
func selfDropInDir(unit string) string {
	return systemdEtcDir + "/" + unit + ".d"
}

// selfStepFunc is the shell helper a mutating script uses: it runs one command
// and, when it fails, emits which step failed and why, and stops.
const selfStepFunc = `step() { name="$1"; shift; out="$("$@" 2>&1)" || { emit failed "$name"; emit err "$(printf %s "$out" | b64)"; exit 0; }; }
`

// selfScript builds the script and its budget for one request, and an empty
// script for an operation it does not know.
func selfScript(req coreagent.HostServicePayload) (string, time.Duration) {
	var b strings.Builder
	b.WriteString(selfPrelude)
	unit := req.Unit
	switch req.Op {
	case coreagent.HostServiceOpList:
		return selfListScript(), selfQueryTimeout
	case coreagent.HostServiceOpAction:
		if req.Action == coreagent.HostServiceActionRestart && RestartsMcHarbor(unit) {
			b.WriteString(selfCapture("action", "systemctl --no-ask-password --no-block restart -- "+unit))
		} else {
			b.WriteString(selfCapture("action", "systemctl --no-ask-password "+req.Action+" -- "+unit))
		}
		return b.String(), selfActionTimeout
	case coreagent.HostServiceOpOverrideGet:
		b.WriteString(selfCapture("effective", "systemctl cat --no-pager -- "+unit))
		fmt.Fprintf(&b, "f=\"%s/override.conf\"\n", selfDropInDir(unit))
		b.WriteString(`if [ -f "$f" ]; then emit exists 1; size="$(wc -c < "$f" | tr -d " ")"; emit size "$size"; ` +
			`if [ "$size" -le ` + strconv.Itoa(overrideMaxBytes) + ` ]; then emit override "$(b64 < "$f")"; fi; fi` + "\n")
		return b.String(), selfQueryTimeout
	case coreagent.HostServiceOpOverridePut:
		b.WriteString(selfStepFunc)
		fmt.Fprintf(&b, "d=\"%s\"\nt=\"\"\n", selfDropInDir(unit))
		b.WriteString(`trap "[ -n \"\$t\" ] && rm -f \"\$t\"" EXIT` + "\n")
		b.WriteString(`step mkdir mkdir -p "$d"` + "\n")
		b.WriteString(`t="$(mktemp "$d/.override.conf.XXXXXX")" || { emit failed mktemp; exit 0; }` + "\n")
		fmt.Fprintf(&b, "printf %%s \"%s\" | base64 -d > \"$t\" || { emit failed write; exit 0; }\n", base64.StdEncoding.EncodeToString([]byte(req.Content)))
		b.WriteString(`step chmod chmod 0644 "$t"` + "\n")
		b.WriteString(`step chown chown 0:0 "$t"` + "\n")
		b.WriteString(`step rename mv -f "$t" "$d/override.conf"` + "\n")
		b.WriteString("t=\"\"\nemit written 1\n")
		if req.Reload {
			b.WriteString(selfCapture("reload", "systemctl daemon-reload"))
		}
		if req.Verify {
			b.WriteString(`frag="$(systemctl show -p FragmentPath --value -- ` + unit + ` 2>/dev/null)"` + "\n")
			b.WriteString(`if [ -n "$frag" ]; then ` + selfCapture("verify", `systemd-analyze verify -- "$frag"`) + "fi\n")
		}
		return b.String(), selfOverrideTimeout
	case coreagent.HostServiceOpOverrideDel:
		b.WriteString(selfStepFunc)
		fmt.Fprintf(&b, "d=\"%s\"\n", selfDropInDir(unit))
		b.WriteString(`step remove rm -f "$d/override.conf"` + "\n")
		b.WriteString(`if [ -d "$d" ] && [ -z "$(ls -A "$d")" ]; then step rmdir rmdir "$d"; fi` + "\n")
		b.WriteString(selfCapture("reload", "systemctl daemon-reload"))
		return b.String(), selfOverrideTimeout
	case coreagent.HostServiceOpDaemonReload:
		b.WriteString(selfCapture("reload", "systemctl daemon-reload"))
		return b.String(), selfReloadTimeout
	}
	return "", 0
}

// parseSelfOutput collects the marker lines into key → value. The utility
// container runs with a TTY, which turns every newline into CRLF.
func parseSelfOutput(out string) map[string]string {
	fields := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		idx := strings.Index(line, selfMarker)
		if idx < 0 {
			continue
		}
		key, value, _ := strings.Cut(line[idx+len(selfMarker):], " ")
		fields[key] = value
	}
	return fields
}

// decoded returns a base64 field as text, or "" when it is absent or garbled.
func decoded(fields map[string]string, key string) string {
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(fields[key]))
	if err != nil {
		return ""
	}
	return string(data)
}

// selfFirstLines trims host output to what fits in an error the operator reads.
func selfFirstLines(text string) string {
	text = strings.TrimSpace(text)
	if len(text) > 2000 {
		text = text[:2000] + "…"
	}
	return text
}

// HostService performs one operation on McHarbor's own machine. A host that
// cannot be reached answers with a Refusal carrying the reason; every other
// refusal comes back as ErrorCode plus a sentence, exactly as an agent answers.
func (l *selfLink) HostService(ctx context.Context, req coreagent.HostServicePayload) (*coreagent.HostServicePayload, error) {
	reply := &coreagent.HostServicePayload{Op: req.Op, Unit: req.Unit, Action: req.Action}
	fail := func(code, message string) (*coreagent.HostServicePayload, error) {
		reply.ErrorCode = code
		reply.Error = message
		return reply, nil
	}
	if req.Op != coreagent.HostServiceOpList && req.Op != coreagent.HostServiceOpDaemonReload {
		if ValidateUnit(req.Unit) != nil {
			return fail("invalid_unit", fmt.Sprintf("%q is not a valid .service unit name", req.Unit))
		}
	}
	switch req.Op {
	case coreagent.HostServiceOpAction:
		if !ValidAction(req.Action) {
			return fail("invalid_action", fmt.Sprintf("unknown action %q", req.Action))
		}
		if actionRefusedBy(SelfProtection, req.Unit, req.Action) {
			return fail("protected", fmt.Sprintf("%s is protected on the machine McHarbor runs on", req.Unit))
		}
	case coreagent.HostServiceOpOverridePut:
		if len(req.Content) > overrideMaxBytes || !utf8.ValidString(req.Content) || strings.ContainsRune(req.Content, 0) {
			return fail("invalid_content", "the override is not UTF-8 text of at most 64 KiB")
		}
	}

	script, timeout := selfScript(req)
	if script == "" {
		return fail("invalid_action", fmt.Sprintf("unknown host service operation %q", req.Op))
	}
	if strings.Contains(script, "'") {
		return nil, errors.New("self host script must not contain a single quote")
	}
	result, err := l.run(ctx, "chroot /host sh -c '"+script+"'", true, timeout)
	if err != nil {
		if reason, ok := l.unreachable(err); ok {
			return nil, &Refusal{Status: http.StatusConflict, Code: i18n.ErrHostServicesUnsupported, Detail: reason, unreachable: true}
		}
		return nil, fmt.Errorf("running on McHarbor's own host: %w", err)
	}
	fields := parseSelfOutput(result.Output)
	if _, ok := fields["nosystemd"]; ok {
		return fail("unsupported", "the machine McHarbor runs on is not running systemd")
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("the self host answered nothing McHarbor could read (exit %d): %s", result.ExitCode, selfFirstLines(result.Output))
	}
	if step, ok := fields["failed"]; ok {
		return fail("failed", fmt.Sprintf("the %s step failed: %s", step, orText(decoded(fields, "err"), "no output")))
	}
	return interpretSelf(req, fields, reply)
}

// orText returns text, or fallback when text is blank.
func orText(text, fallback string) string {
	if text = selfFirstLines(text); text != "" {
		return text
	}
	return fallback
}

// interpretSelf fills reply from one op's marker fields.
func interpretSelf(req coreagent.HostServicePayload, fields map[string]string, reply *coreagent.HostServicePayload) (*coreagent.HostServicePayload, error) {
	failed := func(key string) bool { rc, ok := fields[key+"_rc"]; return ok && rc != "0" }
	fail := func(message string) (*coreagent.HostServicePayload, error) {
		reply.ErrorCode = "failed"
		reply.Error = message
		return reply, nil
	}
	switch req.Op {
	case coreagent.HostServiceOpList:
		units, err := parseListUnitsJSON([]byte(decoded(fields, "units_json")))
		if err != nil || failed("units_json") {
			if failed("units_plain") {
				return fail("systemctl list-units failed: " + orText(decoded(fields, "units_plain"), "exit "+fields["units_plain_rc"]))
			}
			units = parseListUnitsPlain([]byte(decoded(fields, "units_plain")))
		}
		files, err := parseUnitFilesJSON([]byte(decoded(fields, "files_json")))
		if err != nil || failed("files_json") {
			files = map[string]string{}
			if !failed("files_plain") {
				files = parseUnitFilesPlain([]byte(decoded(fields, "files_plain")))
			}
		}
		show := map[string]map[string]string{}
		if !failed("show") {
			show = parseShowBlocks([]byte(decoded(fields, "show")))
		}
		reply.Units = mergeServices(units, files, show)
	case coreagent.HostServiceOpAction:
		if failed("action") {
			return fail(fmt.Sprintf("systemctl %s %s failed: %s", req.Action, req.Unit, orText(decoded(fields, "action"), "exit "+fields["action_rc"])))
		}
		if req.Action == coreagent.HostServiceActionRestart && RestartsMcHarbor(req.Unit) {
			reply.Warning = WarningMcHarborRestart
		}
	case coreagent.HostServiceOpOverrideGet:
		if failed("effective") {
			return fail(fmt.Sprintf("systemctl cat %s failed: %s", req.Unit, orText(decoded(fields, "effective"), "exit "+fields["effective_rc"])))
		}
		reply.Effective = decoded(fields, "effective")
		reply.OverridePath = selfDropInDir(req.Unit) + "/override.conf"
		if _, ok := fields["exists"]; ok {
			reply.OverrideExists = true
			if size, err := strconv.Atoi(fields["size"]); err == nil && size > overrideMaxBytes {
				return fail(fmt.Sprintf("%s is larger than %d bytes", reply.OverridePath, overrideMaxBytes))
			}
			reply.Override = decoded(fields, "override")
		}
	case coreagent.HostServiceOpOverridePut:
		reply.OverridePath = selfDropInDir(req.Unit) + "/override.conf"
		reply.OverrideExists = true
		if req.Reload {
			if failed("reload") {
				return fail("the override was saved but daemon-reload failed: " + orText(decoded(fields, "reload"), "exit "+fields["reload_rc"]))
			}
			reply.Reloaded = true
		}
		if req.Verify {
			reply.VerifyOutput = selfFirstLines(decoded(fields, "verify"))
		}
	case coreagent.HostServiceOpOverrideDel:
		if failed("reload") {
			return fail("the override was removed but daemon-reload failed: " + orText(decoded(fields, "reload"), "exit "+fields["reload_rc"]))
		}
		reply.Reloaded = true
	case coreagent.HostServiceOpDaemonReload:
		if failed("reload") {
			return fail("systemctl daemon-reload failed: " + orText(decoded(fields, "reload"), "exit "+fields["reload_rc"]))
		}
		reply.Reloaded = true
	}
	reply.Success = true
	return reply, nil
}

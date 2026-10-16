// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package hostservices

import (
	"bufio"
	"bytes"
	"encoding/json"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
)

// The parsers below read the same systemctl output the native agent reads in
// src/agent/host_services.go. The agent is a separate Go module the server
// cannot import, so they are kept in step by hand: a fix to one belongs in both.

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
func parseListUnitsJSON(data []byte) ([]coreagent.HostServiceUnit, error) {
	var rows []listUnitJSON
	if err := json.Unmarshal(bytes.TrimSpace(data), &rows); err != nil {
		return nil, err
	}
	units := make([]coreagent.HostServiceUnit, 0, len(rows))
	for _, row := range rows {
		if !strings.HasSuffix(row.Unit, ".service") {
			continue
		}
		units = append(units, coreagent.HostServiceUnit{
			Name: row.Unit, LoadState: row.Load, ActiveState: row.Active,
			SubState: row.Sub, Description: row.Description,
		})
	}
	return units, nil
}

// parseListUnitsPlain reads `systemctl list-units --plain --no-legend`. Some
// versions still print the failed-unit bullet despite --plain, so it is skipped.
func parseListUnitsPlain(data []byte) []coreagent.HostServiceUnit {
	var units []coreagent.HostServiceUnit
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "●"), "*"))
		fields := strings.Fields(line)
		if len(fields) < 4 || !strings.HasSuffix(fields[0], ".service") {
			continue
		}
		units = append(units, coreagent.HostServiceUnit{
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

// failureReason turns systemd's result properties into the one sentence the
// Services table shows behind a failed unit's error icon.
func failureReason(props map[string]string) string {
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

// mergeServices joins the loaded units, the installed unit files and the show
// properties into one sorted list. A unit file that is not loaded still
// appears, inactive, because "installed but never started" is worth seeing.
func mergeServices(units []coreagent.HostServiceUnit, files map[string]string, show map[string]map[string]string) []coreagent.HostServiceUnit {
	byName := make(map[string]*coreagent.HostServiceUnit, len(units)+len(files))
	for i := range units {
		unit := units[i]
		byName[unit.Name] = &unit
	}
	for name, state := range files {
		if unit, ok := byName[name]; ok {
			unit.UnitFileState = state
			continue
		}
		byName[name] = &coreagent.HostServiceUnit{Name: name, UnitFileState: state, ActiveState: "inactive", SubState: "dead"}
	}
	merged := make([]coreagent.HostServiceUnit, 0, len(byName))
	for _, unit := range byName {
		unit.Template = strings.HasSuffix(unit.Name, "@.service")
		if props, ok := show[unit.Name]; ok {
			if pid, err := strconv.Atoi(props["MainPID"]); err == nil && pid > 0 {
				unit.MainPID = pid
			}
			if unit.ActiveState == "failed" {
				unit.Result = props["Result"]
				unit.FailureReason = failureReason(props)
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

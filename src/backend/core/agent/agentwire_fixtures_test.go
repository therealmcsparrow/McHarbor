// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// agentWireDir holds the golden wire fixtures the compute plugins decode in their own tests.
// Plugins no longer import this package's structs, so these files are the only thing that
// tells a plugin's local wire struct it has drifted from the agent protocol.
var agentWireDir = filepath.Join("..", "..", "..", "contracts", "agentwire")

// fillStamp is the value every RFC3339 string field gets, so a consumer can parse it.
const fillStamp = "2026-09-30T12:00:00Z"

// fill sets every exported field reachable from v to a non-zero value derived from the
// field's name, so a fixture exercises every field and a consumer that decodes one into a
// zero value has a wrong tag or a missing field. Deterministic: the same struct always
// fills the same way.
func fill(v reflect.Value, name string, depth int) {
	if depth > 8 {
		return
	}
	if v.Type() == reflect.TypeFor[json.RawMessage]() {
		v.SetBytes([]byte(`{"filled":true}`))
		return
	}
	if v.Type() == reflect.TypeFor[time.Time]() {
		stamp, err := time.Parse(time.RFC3339, fillStamp)
		if err == nil {
			v.Set(reflect.ValueOf(stamp))
		}
		return
	}
	switch v.Kind() {
	case reflect.String:
		if strings.HasSuffix(name, "At") {
			v.SetString(fillStamp)
		} else {
			v.SetString(name)
		}
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(int64(len(name)))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(uint64(len(name)))
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1.5)
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			v.SetBytes([]byte(name))
			return
		}
		s := reflect.MakeSlice(v.Type(), 1, 1)
		fill(s.Index(0), name, depth+1)
		v.Set(s)
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return
		}
		m := reflect.MakeMap(v.Type())
		elem := reflect.New(v.Type().Elem()).Elem()
		fill(elem, name, depth+1)
		m.SetMapIndex(reflect.ValueOf("key").Convert(v.Type().Key()), elem)
		v.Set(m)
	case reflect.Pointer:
		p := reflect.New(v.Type().Elem())
		fill(p.Elem(), name, depth+1)
		v.Set(p)
	case reflect.Struct:
		for i := range v.NumField() {
			f := v.Type().Field(i)
			if !f.IsExported() || f.Tag.Get("json") == "-" {
				continue
			}
			fill(v.Field(i), f.Name, depth+1)
		}
	}
}

// filled returns a T with every field set by fill.
func filled[T any]() T {
	var out T
	fill(reflect.ValueOf(&out).Elem(), reflect.TypeFor[T]().Name(), 0)
	return out
}

// slotFixtures is one fully populated payload per plugin envelope slot. A slot's struct is
// both the request and the reply body, so one fixture per slot covers both directions.
func slotFixtures() map[PluginSlot]any {
	return map[PluginSlot]any{
		SlotVM:           filled[VMPayload](),
		SlotCT:           filled[CTPayload](),
		SlotNet:          filled[ComputeNetPayload](),
		SlotCompute:      filled[ComputePayload](),
		SlotPool:         filled[ComputePoolPayload](),
		SlotComputeImage: filled[ComputeImagePayload](),
	}
}

// TestAgentWireFixturesAreCurrent is the staleness check for src/contracts/agentwire/slots.
// A protocol struct that gains, loses or renames a field changes its fixture, and this fails
// until the fixture is regenerated with MCHARBOR_UPDATE_AGENTWIRE=1; the plugins' own tests
// then decode the new fixture and show which local wire struct needs the field.
func TestAgentWireFixturesAreCurrent(t *testing.T) {
	update := os.Getenv("MCHARBOR_UPDATE_AGENTWIRE") == "1"
	fixtures := slotFixtures()
	for _, r := range PluginRoutes() {
		if _, ok := fixtures[r.Slot]; !ok {
			t.Errorf("route %s uses slot %q, which has no wire fixture", r.Type, r.Slot)
		}
	}
	for slot, payload := range fixtures {
		want, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, '\n')
		path := filepath.Join(agentWireDir, "slots", string(slot)+".json")
		if update {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, want, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v (regenerate with MCHARBOR_UPDATE_AGENTWIRE=1)", path, err)
		}
		if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
			t.Errorf("%s is stale against core/agent's %s struct; regenerate with MCHARBOR_UPDATE_AGENTWIRE=1", path, slot)
		}
	}
}

// TestFillReachesEveryField guards the generator itself: a fixture with a zero field would
// let a plugin's missing field pass unnoticed.
func TestFillReachesEveryField(t *testing.T) {
	for slot, payload := range slotFixtures() {
		var zero []string
		walkZero(reflect.ValueOf(payload), string(slot), &zero, 0)
		if len(zero) > 0 {
			t.Errorf("slot %s fixture leaves %d fields zero: %v", slot, len(zero), zero)
		}
	}
}

func walkZero(v reflect.Value, path string, zero *[]string, depth int) {
	if depth > 8 {
		return
	}
	if v.IsZero() {
		*zero = append(*zero, path)
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		walkZero(v.Elem(), path, zero, depth+1)
	case reflect.Slice:
		if v.Type().Elem().Kind() != reflect.Uint8 {
			walkZero(v.Index(0), path+"[0]", zero, depth+1)
		}
	case reflect.Struct:
		if v.Type() == reflect.TypeFor[time.Time]() {
			return
		}
		for i := range v.NumField() {
			f := v.Type().Field(i)
			if f.IsExported() && f.Tag.Get("json") != "-" {
				walkZero(v.Field(i), path+"."+f.Name, zero, depth+1)
			}
		}
	}
}

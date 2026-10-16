// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apispec

import (
	"encoding"
	"reflect"
	"strings"
	"time"
)

// schemaBuilder turns Go types into OpenAPI schemas, hoisting every named
// struct into components/schemas so a generated client gets real model types
// rather than a hundred anonymous objects.
type schemaBuilder struct {
	components map[string]any
	building   map[string]bool
}

func newSchemaBuilder() *schemaBuilder {
	return &schemaBuilder{components: map[string]any{}, building: map[string]bool{}}
}

var timeType = reflect.TypeOf(time.Time{})

// textMarshalerType marks types that encode themselves as a JSON string (netip.Addr,
// netip.Prefix, a Docker network.Port), whatever their Go shape looks like.
var textMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()

// schemaFor returns the schema for v, registering named structs as components.
func (b *schemaBuilder) schemaFor(v any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	return b.fromType(reflect.TypeOf(v))
}

func (b *schemaBuilder) fromType(t reflect.Type) map[string]any {
	if t == nil {
		return map[string]any{}
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t == timeType {
		return map[string]any{"type": "string", "format": "date-time"}
	}
	if t.Implements(textMarshalerType) || reflect.PointerTo(t).Implements(textMarshalerType) {
		return map[string]any{"type": "string"}
	}

	switch t.Kind() {
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32:
		return map[string]any{"type": "number", "format": "float"}
	case reflect.Float64:
		// Without a format, generated clients default to float32 and lose precision
		// on every round trip, which surfaces as a perpetual Terraform diff.
		return map[string]any{"type": "number", "format": "double"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return map[string]any{"type": "string", "format": "byte"}
		}
		return map[string]any{"type": "array", "items": b.fromType(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": b.fromType(t.Elem())}
	case reflect.Interface:
		// An `any` field carries no shape; saying so beats inventing one.
		return map[string]any{}
	case reflect.Struct:
		return b.structSchema(t)
	default:
		return map[string]any{}
	}
}

func (b *schemaBuilder) structSchema(t reflect.Type) map[string]any {
	name := t.Name()
	if name == "" {
		return b.inlineStruct(t)
	}
	key := componentName(t)

	// A type already being built is a cycle; the reference closes it.
	if b.building[key] {
		return map[string]any{"$ref": "#/components/schemas/" + key}
	}
	if _, done := b.components[key]; done {
		return map[string]any{"$ref": "#/components/schemas/" + key}
	}

	b.building[key] = true
	schema := b.inlineStruct(t)
	delete(b.building, key)
	b.components[key] = schema

	return map[string]any{"$ref": "#/components/schemas/" + key}
}

func (b *schemaBuilder) inlineStruct(t reflect.Type) map[string]any {
	props := map[string]any{}
	var required []string

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" && !f.Anonymous {
			continue // unexported
		}

		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		parts := strings.Split(tag, ",")
		jsonName := parts[0]
		if jsonName == "" {
			jsonName = f.Name
		}

		// An embedded struct with no json name contributes its fields directly;
		// inlining it here rather than via fromType, which would return a $ref.
		embeddedType := f.Type
		if embeddedType.Kind() == reflect.Pointer {
			embeddedType = embeddedType.Elem()
		}
		if f.Anonymous && parts[0] == "" && embeddedType.Kind() == reflect.Struct {
			embedded := b.inlineStruct(embeddedType)
			if sub, ok := embedded["properties"].(map[string]any); ok {
				for k, v := range sub {
					props[k] = v
				}
				if reqs, ok := embedded["required"].([]string); ok {
					required = append(required, reqs...)
				}
				continue
			}
		}

		if f.PkgPath != "" {
			continue // unexported embedded non-struct, which encoding/json ignores
		}
		props[jsonName] = b.fromType(f.Type)
		if !hasOmitEmpty(parts) && f.Type.Kind() != reflect.Pointer {
			required = append(required, jsonName)
		}
	}

	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// hasOmitEmpty reports whether encoding/json may leave the field out; omitzero counts,
// because the Docker API types use it for netip fields that are absent when unset.
func hasOmitEmpty(parts []string) bool {
	for _, p := range parts[1:] {
		if p == "omitempty" || p == "omitzero" {
			return true
		}
	}
	return false
}

// componentName qualifies a type with its package so two modules' Config types
// do not collide in components/schemas.
func componentName(t reflect.Type) string {
	pkg := t.PkgPath()
	if i := strings.LastIndex(pkg, "/"); i >= 0 {
		pkg = pkg[i+1:]
	}
	if pkg == "" {
		return t.Name()
	}
	return camel(pkg) + "." + t.Name()
}

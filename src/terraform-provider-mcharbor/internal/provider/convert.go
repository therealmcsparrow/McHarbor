// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// optString maps an empty or missing server value to null, so an attribute
// the operator never set does not read back as "" and show a diff.
func optString(v *string) types.String {
	if v == nil || *v == "" {
		return types.StringNull()
	}
	return types.StringValue(*v)
}

// optInt maps a missing or zero server value to null for the same reason.
func optInt(v *int) types.Int64 {
	if v == nil || *v == 0 {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*v))
}

// strPtr sends a configured string and omits a null or unknown one.
func strPtr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

// intPtr sends a configured number and omits a null or unknown one.
func intPtr(v types.Int64) *int {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	n := int(v.ValueInt64())
	return &n
}

// boolPtr sends a configured flag and omits a null or unknown one.
func boolPtr(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

// stringSet builds a set attribute value; a nil slice becomes the empty set,
// because every set this provider exposes defaults to empty rather than null.
func stringSet(values []string) types.Set {
	elems := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elems = append(elems, types.StringValue(v))
	}
	return types.SetValueMust(types.StringType, elems)
}

// stringList builds a list attribute value in the server's order.
func stringList(values []string) types.List {
	elems := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elems = append(elems, types.StringValue(v))
	}
	return types.ListValueMust(types.StringType, elems)
}

// setStrings reads a set attribute into a sorted slice, so request bodies are
// deterministic and never null.
func setStrings(ctx context.Context, s types.Set, diags *diag.Diagnostics) []string {
	out := []string{}
	if s.IsNull() || s.IsUnknown() {
		return out
	}
	diags.Append(s.ElementsAs(ctx, &out, false)...)
	sort.Strings(out)
	return out
}

// stringMap reads a map attribute; nil when the operator left it unset.
func stringMap(ctx context.Context, m types.Map, diags *diag.Diagnostics) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	out := map[string]string{}
	diags.Append(m.ElementsAs(ctx, &out, false)...)
	return out
}

// pathRoot names a top-level attribute for SetAttribute.
func pathRoot(name string) path.Path { return path.Root(name) }

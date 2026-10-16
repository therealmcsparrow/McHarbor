// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// oneOfValidator rejects a value outside a fixed set at plan time, before a
// request McHarbor would refuse is ever sent.
type oneOfValidator struct {
	allowed []string
}

func oneOf(values ...string) validator.String { return oneOfValidator{allowed: values} }

func (v oneOfValidator) Description(_ context.Context) string {
	return "must be one of: " + strings.Join(v.allowed, ", ")
}

func (v oneOfValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v oneOfValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	got := req.ConfigValue.ValueString()
	for _, a := range v.allowed {
		if got == a {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(req.Path, "Invalid value",
		fmt.Sprintf("%q %s", got, v.Description(context.Background())))
}

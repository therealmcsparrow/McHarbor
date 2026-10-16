// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package provider implements the McHarbor Terraform provider on the plugin
// framework, over the client generated from McHarbor's OpenAPI document.
package provider

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/client"
)

// Environment variables the provider falls back to, so credentials can stay
// out of the configuration and out of version control.
const (
	// EnvEndpoint names the McHarbor instance root, e.g. https://mcharbor.example:8705.
	EnvEndpoint = "MCHARBOR_ENDPOINT"
	// EnvAPIKey names the API key the provider authenticates with.
	EnvAPIKey = "MCHARBOR_API_KEY"
)

// requestTimeout bounds every call, because a wedged instance would otherwise
// hold a terraform apply open indefinitely.
const requestTimeout = 60 * time.Second

type mcharborProvider struct {
	version string
}

type providerModel struct {
	Endpoint           types.String `tfsdk:"endpoint"`
	APIKey             types.String `tfsdk:"api_key"`
	InsecureSkipVerify types.Bool   `tfsdk:"insecure_skip_verify"`
}

// New returns the provider factory providerserver.Serve and the tests use.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &mcharborProvider{version: version}
	}
}

func (p *mcharborProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "mcharbor"
	resp.Version = p.version
}

func (p *mcharborProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a McHarbor instance through its HTTP API, authenticated with an API key.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Description: "Instance root URL, for example https://mcharbor.example:8705. Falls back to MCHARBOR_ENDPOINT.",
			},
			"api_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "API key sent as a Bearer token. Falls back to MCHARBOR_API_KEY.",
			},
			"insecure_skip_verify": schema.BoolAttribute{
				Optional:    true,
				Description: "Accept any TLS certificate. McHarbor ships a self-signed certificate by default; prefer trusting its CA over enabling this.",
			},
		},
	}
}

func (p *mcharborProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.Endpoint.IsUnknown() || cfg.APIKey.IsUnknown() {
		return
	}

	endpoint := firstNonEmpty(cfg.Endpoint.ValueString(), os.Getenv(EnvEndpoint))
	apiKey := firstNonEmpty(cfg.APIKey.ValueString(), os.Getenv(EnvAPIKey))
	if endpoint == "" {
		resp.Diagnostics.AddError("Missing McHarbor endpoint", "Set the provider's endpoint argument or "+EnvEndpoint+".")
	}
	if apiKey == "" {
		resp.Diagnostics.AddError("Missing McHarbor API key", "Set the provider's api_key argument or "+EnvAPIKey+".")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	api, err := newClient(endpoint, apiKey, cfg.InsecureSkipVerify.ValueBool(), p.version)
	if err != nil {
		addError(&resp.Diagnostics, "Could not build the McHarbor client", err)
		return
	}
	data := &apiClient{api: api}
	resp.ResourceData = data
	resp.DataSourceData = data
}

func (p *mcharborProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newEnvironmentResource,
		newStackResource,
		newRoleResource,
		newRoleAssignmentResource,
		newAPIKeyResource,
		newSecretResource,
		newScheduleResource,
		newWorkflowResource,
		newPluginResource,
		newUpdatePolicyResource,
		newAlertRuleResource,
		newStorageLocationResource,
		newBackupPlanResource,
	}
}

func (p *mcharborProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newEnvironmentsDataSource,
		newRolesDataSource,
		newPermissionCatalogDataSource,
		newSecretDataSource,
		newContainersDataSource,
		newImagesDataSource,
		newAppCatalogDataSource,
		newPluginCatalogDataSource,
	}
}

// newClient builds the generated client with bearer auth, a user agent that
// names the provider version, and the TLS policy the operator chose.
func newClient(endpoint, apiKey string, insecure bool, version string) (*client.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // operator opt-in for the default self-signed certificate
	}
	httpClient := &http.Client{Transport: transport, Timeout: requestTimeout}

	return client.NewClient(strings.TrimSuffix(endpoint, "/"),
		client.WithHTTPClient(httpClient),
		client.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+apiKey)
			req.Header.Set("User-Agent", "terraform-provider-mcharbor/"+version)
			req.Header.Set("Accept", "application/json")
			return nil
		}),
	)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

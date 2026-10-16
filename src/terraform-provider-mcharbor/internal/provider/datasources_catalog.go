// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/client"
)

// listDataSource is a read-only list of flat objects: one list attribute,
// optional string filters, and a fetch that turns the API answer into rows.
type listDataSource struct {
	c        *apiClient
	name     string
	desc     string
	listAttr string
	fields   map[string]attr.Type
	filters  []string
	fetch    func(ctx context.Context, c *apiClient, filter map[string]string) ([]map[string]attr.Value, error)
}

func (d *listDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.name
}

func (d *listDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	nested := map[string]schema.Attribute{}
	for name, typ := range d.fields {
		if lt, ok := typ.(types.ListType); ok {
			nested[name] = schema.ListAttribute{Computed: true, ElementType: lt.ElemType}
			continue
		}
		switch typ {
		case types.Int64Type:
			nested[name] = schema.Int64Attribute{Computed: true}
		case types.BoolType:
			nested[name] = schema.BoolAttribute{Computed: true}
		default:
			nested[name] = schema.StringAttribute{Computed: true}
		}
	}
	attrs := map[string]schema.Attribute{
		d.listAttr: schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: nested}},
	}
	for _, f := range d.filters {
		attrs[f] = schema.StringAttribute{Optional: true}
	}
	resp.Schema = schema.Schema{Description: d.desc, Attributes: attrs}
}

func (d *listDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	d.c = configureDataSource(req)
}

func (d *listDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	filter := map[string]string{}
	for _, f := range d.filters {
		var v types.String
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, pathRoot(f), &v)...)
		if !v.IsNull() && !v.IsUnknown() {
			filter[f] = v.ValueString()
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, pathRoot(f), v)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	rows, err := d.fetch(ctx, d.c, filter)
	if err != nil {
		addError(&resp.Diagnostics, "Reading "+strings.ReplaceAll(d.name, "_", " "), err)
		return
	}
	objType := types.ObjectType{AttrTypes: d.fields}
	items := make([]attr.Value, 0, len(rows))
	for _, r := range rows {
		items = append(items, types.ObjectValueMust(d.fields, r))
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, pathRoot(d.listAttr), types.ListValueMust(objType, items))...)
}

func optionalParam(filter map[string]string, key string) *string {
	if v, ok := filter[key]; ok && v != "" {
		return &v
	}
	return nil
}

func newContainersDataSource() datasource.DataSource {
	return &listDataSource{
		name: "containers", listAttr: "containers", filters: []string{"environment_id"},
		desc: "Containers in one environment, running and stopped. Requires containers.view.",
		fields: map[string]attr.Type{
			"id": types.StringType, "name": types.StringType, "image": types.StringType,
			"state": types.StringType, "status": types.StringType, "stack": types.StringType,
		},
		fetch: func(ctx context.Context, c *apiClient, f map[string]string) ([]map[string]attr.Value, error) {
			all := "true"
			var list []struct {
				ID        string   `json:"Id"`
				Names     []string `json:"Names"`
				Image     string   `json:"Image"`
				State     string   `json:"State"`
				Status    string   `json:"Status"`
				StackName string   `json:"StackName"`
			}
			if err := decode(apiCall(c.api.ContainersList(ctx, &client.ContainersListParams{Env: optionalParam(f, "environment_id"), All: &all})), "containersList", &list); err != nil {
				return nil, err
			}
			out := make([]map[string]attr.Value, 0, len(list))
			for _, ct := range list {
				name := ""
				if len(ct.Names) > 0 {
					name = strings.TrimPrefix(ct.Names[0], "/")
				}
				out = append(out, map[string]attr.Value{
					"id": types.StringValue(ct.ID), "name": types.StringValue(name), "image": types.StringValue(ct.Image),
					"state": types.StringValue(ct.State), "status": types.StringValue(ct.Status), "stack": types.StringValue(ct.StackName),
				})
			}
			return out, nil
		},
	}
}

func newImagesDataSource() datasource.DataSource {
	return &listDataSource{
		name: "images", listAttr: "images", filters: []string{"environment_id"},
		desc: "Images in one environment. Requires images.view.",
		fields: map[string]attr.Type{
			"id": types.StringType, "tags": types.ListType{ElemType: types.StringType}, "size": types.Int64Type,
		},
		fetch: func(ctx context.Context, c *apiClient, f map[string]string) ([]map[string]attr.Value, error) {
			var list []struct {
				ID       string   `json:"Id"`
				RepoTags []string `json:"RepoTags"`
				Size     int64    `json:"Size"`
			}
			if err := decode(apiCall(c.api.ImagesList(ctx, &client.ImagesListParams{Env: optionalParam(f, "environment_id")})), "imagesList", &list); err != nil {
				return nil, err
			}
			out := make([]map[string]attr.Value, 0, len(list))
			for _, im := range list {
				out = append(out, map[string]attr.Value{
					"id": types.StringValue(im.ID), "tags": stringList(im.RepoTags), "size": types.Int64Value(im.Size),
				})
			}
			return out, nil
		},
	}
}

func newAppCatalogDataSource() datasource.DataSource {
	return &listDataSource{
		name: "app_catalog", listAttr: "apps", filters: []string{"category", "search"},
		desc: "Apps in the enabled app catalogs, optionally by category or search text. Requires store_apps.view.",
		fields: map[string]attr.Type{
			"id": types.StringType, "slug": types.StringType, "name": types.StringType,
			"category": types.StringType, "image": types.StringType, "description": types.StringType,
		},
		fetch: func(ctx context.Context, c *apiClient, f map[string]string) ([]map[string]attr.Value, error) {
			perPage := "1000"
			var page struct {
				Items []struct {
					ID          string `json:"id"`
					Slug        string `json:"slug"`
					Name        string `json:"name"`
					Category    string `json:"category"`
					Image       string `json:"image"`
					Description string `json:"description"`
				} `json:"items"`
			}
			params := &client.AppstoreListParams{PerPage: &perPage, Category: optionalParam(f, "category"), Search: optionalParam(f, "search")}
			if err := decode(apiCall(c.api.AppstoreList(ctx, params)), "appstoreList", &page); err != nil {
				return nil, err
			}
			out := make([]map[string]attr.Value, 0, len(page.Items))
			for _, a := range page.Items {
				out = append(out, map[string]attr.Value{
					"id": types.StringValue(a.ID), "slug": types.StringValue(a.Slug), "name": types.StringValue(a.Name),
					"category": types.StringValue(a.Category), "image": types.StringValue(a.Image), "description": types.StringValue(a.Description),
				})
			}
			return out, nil
		},
	}
}

type catalogPlugin struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Versions    []struct {
		Version string `json:"version"`
	} `json:"versions"`
}

func pluginCatalog(ctx context.Context, c *apiClient) ([]catalogPlugin, error) {
	var list []catalogPlugin
	err := decode(apiCall(c.api.AppstorePluginsListCatalog(ctx)), "appstorePluginsListCatalog", &list)
	return list, err
}

func newPluginCatalogDataSource() datasource.DataSource {
	return &listDataSource{
		name: "plugin_catalog", listAttr: "plugins",
		desc: "Plugins the configured catalogs offer, with every version each one serves. Requires plugins.view.",
		fields: map[string]attr.Type{
			"slug": types.StringType, "name": types.StringType, "latest_version": types.StringType,
			"versions": types.ListType{ElemType: types.StringType}, "description": types.StringType,
		},
		fetch: func(ctx context.Context, c *apiClient, _ map[string]string) ([]map[string]attr.Value, error) {
			list, err := pluginCatalog(ctx, c)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]attr.Value, 0, len(list))
			for _, p := range list {
				versions := make([]string, 0, len(p.Versions))
				for _, v := range p.Versions {
					versions = append(versions, v.Version)
				}
				out = append(out, map[string]attr.Value{
					"slug": types.StringValue(p.Slug), "name": types.StringValue(p.Name), "latest_version": types.StringValue(p.Version),
					"versions": stringList(versions), "description": types.StringValue(p.Description),
				})
			}
			return out, nil
		},
	}
}

package main

import (
	"context"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/mini4141/ground-station-cloud/internal/api"
)

type groundstationProvider struct{}

// New is the provider factory.
func New() provider.Provider { return &groundstationProvider{} }

type providerModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	Token    types.String `tfsdk:"token"`
}

func (p *groundstationProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "groundstation"
}

func (p *groundstationProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages ground stations of the Ground Station Cloud platform.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{Optional: true, Description: "API base URL (env GROUNDSTATION_ENDPOINT)."},
			"token":    schema.StringAttribute{Optional: true, Sensitive: true, Description: "Operator JWT (env GROUNDSTATION_TOKEN)."},
		},
	}
}

func (p *groundstationProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	endpoint, token := os.Getenv("GROUNDSTATION_ENDPOINT"), os.Getenv("GROUNDSTATION_TOKEN")
	if !cfg.Endpoint.IsNull() {
		endpoint = cfg.Endpoint.ValueString()
	}
	if !cfg.Token.IsNull() {
		token = cfg.Token.ValueString()
	}
	if endpoint == "" {
		resp.Diagnostics.AddError("missing endpoint", "set endpoint or GROUNDSTATION_ENDPOINT")
		return
	}
	c, err := api.NewClientWithResponses(endpoint, api.WithRequestEditorFn(func(_ context.Context, r *http.Request) error {
		r.Header.Set("Authorization", "Bearer "+token)
		return nil
	}))
	if err != nil {
		resp.Diagnostics.AddError("client", err.Error())
		return
	}
	resp.ResourceData, resp.DataSourceData = c, c
}

func (p *groundstationProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{NewStationResource}
}

func (p *groundstationProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{NewPassesDataSource}
}

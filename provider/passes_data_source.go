package main

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/mini4141/ground-station-cloud/internal/api"
)

type passesDataSource struct{ c *api.ClientWithResponses }

// NewPassesDataSource is the data source factory.
func NewPassesDataSource() datasource.DataSource { return &passesDataSource{} }

type passModel struct {
	ID              types.String  `tfsdk:"id"`
	StationName     types.String  `tfsdk:"station_name"`
	AOS             types.String  `tfsdk:"aos"`
	LOS             types.String  `tfsdk:"los"`
	MaxElevationDeg types.Float64 `tfsdk:"max_elevation_deg"`
}

type passesModel struct {
	SatelliteID types.String `tfsdk:"satellite_id"`
	Station     types.String `tfsdk:"station"`
	From        types.String `tfsdk:"from"`
	To          types.String `tfsdk:"to"`
	Passes      []passModel  `tfsdk:"passes"`
}

func (d *passesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_passes"
}

func (d *passesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Predicted passes of a satellite over the station network.",
		Attributes: map[string]schema.Attribute{
			"satellite_id": schema.StringAttribute{Required: true},
			"station":      schema.StringAttribute{Optional: true, Description: "Station id or name; all stations when empty."},
			"from":         schema.StringAttribute{Optional: true, Description: "RFC3339 window start (simulated time)."},
			"to":           schema.StringAttribute{Optional: true, Description: "RFC3339 window end."},
			"passes": schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"id":                schema.StringAttribute{Computed: true},
				"station_name":      schema.StringAttribute{Computed: true},
				"aos":               schema.StringAttribute{Computed: true},
				"los":               schema.StringAttribute{Computed: true},
				"max_elevation_deg": schema.Float64Attribute{Computed: true},
			}}},
		},
	}
}

func (d *passesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*api.ClientWithResponses); ok {
		d.c = c
	}
}

func (d *passesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg passesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	params := &api.ListPassesParams{Satellite: cfg.SatelliteID.ValueString()}
	if !cfg.Station.IsNull() {
		params.Station = cfg.Station.ValueStringPointer()
	}
	if t, err := time.Parse(time.RFC3339, cfg.From.ValueString()); err == nil {
		params.From = &t
	}
	if t, err := time.Parse(time.RFC3339, cfg.To.ValueString()); err == nil {
		params.To = &t
	}
	out, err := d.c.ListPassesWithResponse(ctx, params)
	if err != nil || out.JSON200 == nil {
		resp.Diagnostics.AddError("list passes", "request failed")
		return
	}
	cfg.Passes = nil
	for _, p := range *out.JSON200 {
		cfg.Passes = append(cfg.Passes, passModel{ID: types.StringValue(p.Id), StationName: types.StringValue(p.StationName),
			AOS: types.StringValue(p.Aos.Format(time.RFC3339)), LOS: types.StringValue(p.Los.Format(time.RFC3339)), MaxElevationDeg: types.Float64Value(p.MaxElevationDeg)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

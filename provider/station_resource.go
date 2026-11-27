package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/mini4141/ground-station-cloud/internal/api"
)

type stationResource struct{ c *api.ClientWithResponses }

// NewStationResource is the resource factory.
func NewStationResource() resource.Resource { return &stationResource{} }

type stationModel struct {
	ID              types.String  `tfsdk:"id"`
	Name            types.String  `tfsdk:"name"`
	Latitude        types.Float64 `tfsdk:"latitude"`
	Longitude       types.Float64 `tfsdk:"longitude"`
	AltitudeM       types.Float64 `tfsdk:"altitude_m"`
	MinElevationDeg types.Float64 `tfsdk:"min_elevation_deg"`
	Band            types.String  `tfsdk:"band"`
	RatePerMinMinor types.Int64   `tfsdk:"rate_per_min_minor"`
}

func (r *stationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_station"
}

func (r *stationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A ground station registered on the platform.",
		Attributes: map[string]schema.Attribute{
			"id":                 schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name":               schema.StringAttribute{Required: true, Description: "Slug; also MQTT topic key and IoT thing name.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"latitude":           schema.Float64Attribute{Required: true},
			"longitude":          schema.Float64Attribute{Required: true},
			"altitude_m":         schema.Float64Attribute{Optional: true, Computed: true, Default: float64default.StaticFloat64(0)},
			"min_elevation_deg":  schema.Float64Attribute{Optional: true, Computed: true, Default: float64default.StaticFloat64(5)},
			"band":               schema.StringAttribute{Required: true},
			"rate_per_min_minor": schema.Int64Attribute{Required: true},
		},
	}
}

func (r *stationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*api.ClientWithResponses); ok {
		r.c = c
	}
}

func fromAPI(s *api.Station) stationModel {
	m := stationModel{ID: types.StringValue(s.Id), Name: types.StringValue(s.Name), Latitude: types.Float64Value(s.Latitude),
		Longitude: types.Float64Value(s.Longitude), Band: types.StringValue(s.Band), RatePerMinMinor: types.Int64Value(int64(s.RatePerMinMinor)),
		AltitudeM: types.Float64Value(0), MinElevationDeg: types.Float64Value(5)}
	if s.AltitudeM != nil {
		m.AltitudeM = types.Float64Value(*s.AltitudeM)
	}
	if s.MinElevationDeg != nil {
		m.MinElevationDeg = types.Float64Value(*s.MinElevationDeg)
	}
	return m
}

func (r *stationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan stationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	alt, minEl := plan.AltitudeM.ValueFloat64(), plan.MinElevationDeg.ValueFloat64()
	out, err := r.c.CreateStationWithResponse(ctx, api.StationInput{Name: plan.Name.ValueString(), Latitude: plan.Latitude.ValueFloat64(),
		Longitude: plan.Longitude.ValueFloat64(), AltitudeM: &alt, MinElevationDeg: &minEl, Band: plan.Band.ValueString(), RatePerMinMinor: int32(plan.RatePerMinMinor.ValueInt64())})
	if err != nil || out.JSON201 == nil {
		resp.Diagnostics.AddError("create station", fmt.Sprintf("%v %s", err, statusOf(out)))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, fromAPI(out.JSON201))...)
}

func statusOf(out interface{ StatusCode() int }) string {
	if out == nil {
		return ""
	}
	return http.StatusText(out.StatusCode())
}

func (r *stationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state stationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	out, err := r.c.GetStationWithResponse(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("read station", err.Error())
		return
	}
	if out.StatusCode() == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if out.JSON200 == nil {
		resp.Diagnostics.AddError("read station", statusOf(out))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, fromAPI(out.JSON200))...)
}

func (r *stationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state stationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	lat, lon, alt, minEl, band, rate := plan.Latitude.ValueFloat64(), plan.Longitude.ValueFloat64(), plan.AltitudeM.ValueFloat64(),
		plan.MinElevationDeg.ValueFloat64(), plan.Band.ValueString(), int32(plan.RatePerMinMinor.ValueInt64())
	out, err := r.c.UpdateStationWithResponse(ctx, state.ID.ValueString(), api.StationPatch{Latitude: &lat, Longitude: &lon, AltitudeM: &alt,
		MinElevationDeg: &minEl, Band: &band, RatePerMinMinor: &rate})
	if err != nil || out.JSON200 == nil {
		resp.Diagnostics.AddError("update station", fmt.Sprintf("%v %s", err, statusOf(out)))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, fromAPI(out.JSON200))...)
}

func (r *stationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state stationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	out, err := r.c.DeleteStationWithResponse(ctx, state.ID.ValueString())
	if err != nil || (out.StatusCode() != http.StatusNoContent && out.StatusCode() != http.StatusNotFound) {
		resp.Diagnostics.AddError("delete station", fmt.Sprintf("%v %s", err, statusOf(out)))
	}
}

func (r *stationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

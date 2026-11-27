package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

var factories = map[string]func() (tfprotov6.ProviderServer, error){
	"groundstation": providerserver.NewProtocol6WithError(New()),
}

// Acceptance test: TF_ACC=1 GROUNDSTATION_ENDPOINT=http://localhost:8080 GROUNDSTATION_TOKEN=<jwt> go test ./provider
func TestAccStation(t *testing.T) {
	if os.Getenv("GROUNDSTATION_ENDPOINT") == "" {
		t.Skip("GROUNDSTATION_ENDPOINT not set")
	}
	name := fmt.Sprintf("acc-%d", time.Now().Unix())
	cfg := func(rate int) string {
		return fmt.Sprintf(`
resource "groundstation_station" "t" {
  name               = %q
  latitude           = 35.68
  longitude          = 139.69
  band               = "S"
  rate_per_min_minor = %d
}`, name, rate)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg(100), Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("groundstation_station.t", "name", name),
				resource.TestCheckResourceAttrSet("groundstation_station.t", "id"),
				resource.TestCheckResourceAttr("groundstation_station.t", "min_elevation_deg", "5"),
			)},
			{Config: cfg(150), Check: resource.TestCheckResourceAttr("groundstation_station.t", "rate_per_min_minor", "150")}, // in-place update
			{ResourceName: "groundstation_station.t", ImportState: true, ImportStateVerify: true},
		},
	})
}

// Command terraform-provider-groundstation manages ground stations through the platform API.
package main

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

func main() {
	err := providerserver.Serve(context.Background(), New, providerserver.ServeOpts{Address: "registry.terraform.io/mini4141/groundstation"})
	if err != nil {
		log.Fatal(err)
	}
}

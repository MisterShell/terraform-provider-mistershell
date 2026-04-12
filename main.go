package main

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"terraform-provider-mistershell/internal/provider"
)

var version string = "dev"

func main() {
	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/mistershell/mistershell",
	}
	err := providerserver.Serve(context.Background(), provider.New(version), opts)
	if err != nil {
		log.Fatal(err.Error())
	}
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Command terraform-provider-mcharbor serves the McHarbor Terraform provider
// over the plugin protocol Terraform launches it with.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/provider"
)

// version is stamped at release with -ldflags "-X main.version=<calendar version>";
// the provider follows its own sequence, not McHarbor's.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/therealmcsparrow/mcharbor",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}

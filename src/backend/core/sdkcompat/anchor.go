// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package sdkcompat anchors third-party SDK packages as direct
// dependencies of the host module. The host itself does not call
// the AWS SDK at runtime — every storage-s3 operation lives in the
// storage-s3 plugin's compiled .so. But the plugin builds against
// the host's module graph (see docker/Dockerfile line 195 — the
// host module's go.mod governs the plugin's compilation so the
// resulting .so links against identical package versions), and Go
// plugin mode only resolves packages that appear in the module's
// require block. With these imports marked // indirect by go mod
// tidy (no host code uses them), the plugin's `go build` fails
// with "no required module provides package
// github.com/aws/aws-sdk-go-v2/config".
//
// The blank-import anchor keeps `go mod tidy` from pruning the
// packages while adding zero runtime cost — the linker drops the
// empty import records from the final binary. New storage
// plugins that pull in their own SDKs should add a corresponding
// blank import here so the same build path keeps working.
package sdkcompat

import (
	// Anchor the AWS SDK v2 modules used by storage-s3 so the
	// plugin's `-buildmode=plugin` build resolves them through
	// the host module graph.
	_ "github.com/aws/aws-sdk-go-v2/aws"
	_ "github.com/aws/aws-sdk-go-v2/config"
	_ "github.com/aws/aws-sdk-go-v2/credentials"
	_ "github.com/aws/aws-sdk-go-v2/service/s3"
	_ "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

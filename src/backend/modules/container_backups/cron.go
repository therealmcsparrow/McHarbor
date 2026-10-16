// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"time"

	"github.com/therealmcsparrow/mcharbor/core/cronspec"
)

func cronMatches(spec string, now time.Time) bool { return cronspec.Matches(spec, now) }

func validCronSpec(spec string) bool { return cronspec.Valid(spec) }

func nextCronRun(spec string, from time.Time) time.Time { return cronspec.Next(spec, from) }

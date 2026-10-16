// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "strings"

// Paper.
//
// A report is exported to a physical page, and the two papers that matter
// are the ISO one the rest of the world prints on and the US legal sheet
// a board pack is still occasionally asked for. Both writers take the
// size as a parameter rather than baking A4 in, so a wide operational
// table has somewhere to go without changing the type scale.

type pageSize struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Width  float64 `json:"width"`  // points
	Height float64 `json:"height"` // points
}

var (
	pageA4    = pageSize{Key: "a4", Label: "A4", Width: 595.276, Height: 841.890}
	pageLegal = pageSize{Key: "legal", Label: "Legal", Width: 612, Height: 1008}
)

// pageSizes is the set offered to the studio, in menu order.
func pageSizes() []pageSize { return []pageSize{pageA4, pageLegal} }

// pageSizeByKey resolves a query value. An empty key means "the default",
// which is A4; anything else unrecognised is a client error rather than a
// silent fallback, because a board pack printed on the wrong paper is a
// worse outcome than a refused download.
func pageSizeByKey(key string) (pageSize, bool) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "", pageA4.Key:
		return pageA4, true
	case pageLegal.Key:
		return pageLegal, true
	default:
		return pageSize{}, false
	}
}

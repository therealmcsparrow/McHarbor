// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import "strconv"

// pageSize is the largest page McHarbor serves (response.ParsePagination caps at 1000).
const pageSize = 1000

// page is the data shape of every paginated McHarbor list.
type page[T any] struct {
	Items      []T `json:"items"`
	TotalPages int `json:"total_pages"`
}

// listAll follows a paginated list to its end, because a read that stops at
// page one silently loses objects once an instance grows.
func listAll[T any](operation string, fetch func(page, perPage string) call) ([]T, error) {
	var out []T
	for n := 1; ; n++ {
		var p page[T]
		if err := decode(fetch(strconv.Itoa(n), strconv.Itoa(pageSize)), operation, &p); err != nil {
			return nil, err
		}
		out = append(out, p.Items...)
		if n >= p.TotalPages || len(p.Items) == 0 {
			return out, nil
		}
	}
}

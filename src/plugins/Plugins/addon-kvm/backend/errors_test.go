// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"fmt"
	"testing"
)

// TestSentinelDetailCarriesOnlyTheServiceSentence guards the resize and backup
// chain refusals: the client gets the sentence the service wrote after the
// sentinel, never the text of an error wrapped beneath it.
func TestSentinelDetailCarriesOnlyTheServiceSentence(t *testing.T) {
	err := fmt.Errorf("%w: above the ceiling of %d MB this guest was started with", ErrInvalidResize, 4096)
	if got, want := sentinelDetail(err, ErrInvalidResize), "Above the ceiling of 4096 MB this guest was started with"; got != want {
		t.Fatalf("detail = %q, want %q", got, want)
	}

	if got := sentinelDetail(ErrChainIncomplete, ErrChainIncomplete); got != "" {
		t.Fatalf("a bare sentinel carries no detail, got %q", got)
	}
	if got := sentinelDetail(errors.New("dial tcp 10.0.0.5: refused"), ErrInvalidResize); got != "" {
		t.Fatalf("an unrelated error must yield no detail, got %q", got)
	}
	if got := sentinelDetail(nil, ErrInvalidResize); got != "" {
		t.Fatalf("nil must yield no detail, got %q", got)
	}
}

// TestIsValidationErrorClassifiesByText pins the markers the handlers rely on
// to answer 400 instead of 502.
func TestIsValidationErrorClassifiesByText(t *testing.T) {
	if !isValidationError(errors.New("a name is required")) {
		t.Fatal("a required-field message must classify as validation")
	}
	if isValidationError(errors.New("connection refused")) || isValidationError(nil) {
		t.Fatal("transport errors and nil must not classify as validation")
	}
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"errors"
	"testing"
)

func TestRequireNetSuccessKeepsTheHostLinkCodes(t *testing.T) {
	cases := []struct {
		code string
		want error
	}{
		{"uplink_busy", ErrUplinkBusy},
		{HostLinkErrNameTaken, ErrHostLinkNameTaken},
		{HostLinkErrParentNotFound, ErrHostLinkParentNotFound},
		{HostLinkErrPermissionDenied, ErrHostLinkPermissionDenied},
	}
	for _, tc := range cases {
		reply := &WSMessage{Net: &ComputeNetPayload{Success: false, Error: "refused", ErrorCode: tc.code}}
		if err := requireNetSuccess(reply, "ensure network"); !errors.Is(err, tc.want) {
			t.Errorf("code %q: got %v, want %v", tc.code, err, tc.want)
		}
	}
}

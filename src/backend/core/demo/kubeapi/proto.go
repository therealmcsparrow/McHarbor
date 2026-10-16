// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package kubeapi

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var protoMagic = []byte{'k', '8', 's', 0}

var errNoReplicas = errors.New("scale body carries no spec.replicas")

// decodeScaleReplicas reads spec.replicas from an autoscaling/v1 Scale body.
// client-go sends it as protobuf by default, and the backend has no generated
// k8s types, so the envelope and the two nested messages are walked by hand.
func decodeScaleReplicas(contentType string, body []byte) (int32, error) {
	if strings.Contains(contentType, "protobuf") || bytes.HasPrefix(body, protoMagic) {
		if !bytes.HasPrefix(body, protoMagic) {
			return 0, errors.New("protobuf body without the k8s magic prefix")
		}
		raw, err := protoField(body[len(protoMagic):], 2)
		if err != nil {
			return 0, fmt.Errorf("reading runtime.Unknown raw: %w", err)
		}
		spec, err := protoField(raw, 2)
		if err != nil {
			return 0, nil //nolint:nilerr // a scale body without a spec.replicas field means zero replicas, not a malformed request
		}
		return protoVarintField(spec, 1)
	}
	var in struct {
		Spec *struct {
			Replicas *int32 `json:"replicas"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		return 0, fmt.Errorf("decoding scale json: %w", err)
	}
	if in.Spec == nil || in.Spec.Replicas == nil {
		return 0, errNoReplicas
	}
	return *in.Spec.Replicas, nil
}

// protoField returns the bytes of the first length-delimited field num.
func protoField(msg []byte, num uint64) ([]byte, error) {
	for len(msg) > 0 {
		key, n := binary.Uvarint(msg)
		if n <= 0 {
			return nil, errors.New("bad protobuf key")
		}
		msg = msg[n:]
		field, wire := key>>3, key&7
		switch wire {
		case 0:
			_, m := binary.Uvarint(msg)
			if m <= 0 {
				return nil, errors.New("bad protobuf varint")
			}
			msg = msg[m:]
		case 1:
			if len(msg) < 8 {
				return nil, errors.New("short fixed64")
			}
			msg = msg[8:]
		case 5:
			if len(msg) < 4 {
				return nil, errors.New("short fixed32")
			}
			msg = msg[4:]
		case 2:
			l, m := binary.Uvarint(msg)
			if m <= 0 || uint64(len(msg)-m) < l {
				return nil, errors.New("bad protobuf length")
			}
			val := msg[m : m+int(l)]
			msg = msg[m+int(l):]
			if field == num {
				return val, nil
			}
		default:
			return nil, fmt.Errorf("unsupported protobuf wire type %d", wire)
		}
	}
	return nil, fmt.Errorf("protobuf field %d not present", num)
}

// protoVarintField returns varint field num as an int32; an absent field is the
// proto2 zero value, which is how a Scale to zero replicas can arrive.
func protoVarintField(msg []byte, num uint64) (int32, error) {
	for len(msg) > 0 {
		key, n := binary.Uvarint(msg)
		if n <= 0 {
			return 0, errors.New("bad protobuf key")
		}
		msg = msg[n:]
		field, wire := key>>3, key&7
		switch wire {
		case 0:
			v, m := binary.Uvarint(msg)
			if m <= 0 {
				return 0, errors.New("bad protobuf varint")
			}
			msg = msg[m:]
			if field == num {
				return int32(v), nil
			}
		case 1:
			if len(msg) < 8 {
				return 0, errors.New("short fixed64")
			}
			msg = msg[8:]
		case 5:
			if len(msg) < 4 {
				return 0, errors.New("short fixed32")
			}
			msg = msg[4:]
		case 2:
			l, m := binary.Uvarint(msg)
			if m <= 0 || uint64(len(msg)-m) < l {
				return 0, errors.New("bad protobuf length")
			}
			msg = msg[m+int(l):]
		default:
			return 0, fmt.Errorf("unsupported protobuf wire type %d", wire)
		}
	}
	return 0, nil
}

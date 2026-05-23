// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package customnodes

import (
	"context"
	"fmt"
)

// Bridge wraps Executor to satisfy the workflow service's custom executor interface.
// This avoids a circular import between workflows and custom_nodes packages.
type Bridge struct {
	executor *Executor
}

// NewBridge creates a bridge between the custom node executor and the workflow service.
func NewBridge(executor *Executor) *Bridge {
	return &Bridge{executor: executor}
}

// ExecuteCustom runs a custom node script and returns (port, msg, error).
// If nodeKey is empty, it reads inline code from config["code"] (used by node-script).
func (b *Bridge) ExecuteCustom(ctx context.Context, nodeKey string, config, msg map[string]interface{}, timeout float64) (string, map[string]interface{}, error) {
	var result *ExecResult
	var err error

	if nodeKey == "" {
		// Inline script mode (node-script built-in node)
		code, _ := config["code"].(string)
		if code == "" {
			return "error", nil, fmt.Errorf("no script code provided")
		}
		result, err = b.executor.RunScript(ctx, code, config, msg, timeout)
	} else {
		// Custom node mode — load script from disk
		result, err = b.executor.Execute(ctx, nodeKey, config, msg, timeout)
	}

	if err != nil {
		if result != nil {
			return result.Port, result.Msg, err
		}
		return "error", nil, err
	}
	return result.Port, result.Msg, nil
}

// IsCustomNode checks if the given key corresponds to a custom node.
func (b *Bridge) IsCustomNode(key string) bool {
	return b.executor.IsCustomNode(key)
}

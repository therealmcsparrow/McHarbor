// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package custom_nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// maxScriptTimeout is the absolute maximum execution time for a custom node script.
const maxScriptTimeout = 60 * time.Second

// maxLogLines caps the number of log lines captured per execution.
const maxLogLines = 100

const (
	customNodeScriptTimedOut      = "script timed out"
	customNodeScriptFailed        = "script execution failed"
	customNodeInvalidScriptResult = "script returned an invalid result"
)

// Executor runs custom node JavaScript in a sandboxed goja VM.
type Executor struct {
	service *Service
	logger  *slog.Logger
}

// NewExecutor creates a new custom node executor.
func NewExecutor(svc *Service, logger *slog.Logger) *Executor {
	return &Executor{service: svc, logger: logger}
}

// ExecResult is the output of running a custom node script.
type ExecResult struct {
	Port string
	Msg  map[string]interface{}
	Logs []string
}

// Execute runs the script for a custom node with the given config and message.
func (e *Executor) Execute(ctx context.Context, nodeKey string, config map[string]interface{}, msg map[string]interface{}, timeoutSec float64) (*ExecResult, error) {
	code, err := e.service.LoadCode(nodeKey)
	if err != nil {
		return nil, fmt.Errorf("loading script for %s: %w", nodeKey, err)
	}

	return e.RunScript(ctx, code, config, msg, timeoutSec)
}

// RunScript executes arbitrary JavaScript code with config and msg inputs.
// Used by both custom node execution and the test endpoint.
func (e *Executor) RunScript(ctx context.Context, code string, config map[string]interface{}, msg map[string]interface{}, timeoutSec float64) (*ExecResult, error) {
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	if timeoutSec > 60 {
		timeoutSec = 60
	}
	timeout := time.Duration(timeoutSec * float64(time.Second))

	vm := goja.New()

	// Capture logs
	var mu sync.Mutex
	var logs []string
	addLog := func(level, message string) {
		mu.Lock()
		defer mu.Unlock()
		if len(logs) < maxLogLines {
			logs = append(logs, fmt.Sprintf("[%s] %s", level, message))
		}
	}

	// Set up the node helper object
	nodeObj := vm.NewObject()
	_ = nodeObj.Set("log", func(call goja.FunctionCall) goja.Value {
		addLog("info", formatArgs(call))
		return goja.Undefined()
	})
	_ = nodeObj.Set("warn", func(call goja.FunctionCall) goja.Value {
		addLog("warn", formatArgs(call))
		return goja.Undefined()
	})
	_ = nodeObj.Set("error", func(call goja.FunctionCall) goja.Value {
		addLog("error", formatArgs(call))
		return goja.Undefined()
	})

	// Set globals
	vm.Set("node", nodeObj)
	vm.Set("config", config)
	vm.Set("msg", msg)

	// console.log support
	consoleObj := vm.NewObject()
	_ = consoleObj.Set("log", func(call goja.FunctionCall) goja.Value {
		addLog("info", formatArgs(call))
		return goja.Undefined()
	})
	_ = consoleObj.Set("warn", func(call goja.FunctionCall) goja.Value {
		addLog("warn", formatArgs(call))
		return goja.Undefined()
	})
	_ = consoleObj.Set("error", func(call goja.FunctionCall) goja.Value {
		addLog("error", formatArgs(call))
		return goja.Undefined()
	})
	vm.Set("console", consoleObj)

	// Timeout via context + interrupt
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Interrupt the VM when context expires
	go func() {
		<-execCtx.Done()
		vm.Interrupt("execution timeout")
	}()

	// Wrap user code in an IIFE so return works
	wrapped := "(function() {\n" + code + "\n})()"

	val, err := vm.RunString(wrapped)
	if err != nil {
		if interrupted, ok := err.(*goja.InterruptedError); ok {
			return &ExecResult{
				Port: "error",
				Msg: map[string]interface{}{
					"payload": map[string]interface{}{"error": customNodeScriptTimedOut},
				},
				Logs: logs,
			}, fmt.Errorf("script interrupted: %s", interrupted.Value())
		}
		e.logger.Warn("custom-nodes: script execution failed", "error", err)
		return &ExecResult{
			Port: "error",
			Msg: map[string]interface{}{
				"payload": map[string]interface{}{"error": customNodeScriptFailed},
			},
			Logs: logs,
		}, fmt.Errorf("script error: %w", err)
	}

	// Parse the return value
	result, err := parseReturnValue(vm, val, msg)
	if err != nil {
		e.logger.Warn("custom-nodes: invalid script result", "error", err)
		return &ExecResult{
			Port: "error",
			Msg: map[string]interface{}{
				"payload": map[string]interface{}{"error": customNodeInvalidScriptResult},
			},
			Logs: logs,
		}, nil
	}

	result.Logs = logs
	return result, nil
}

// IsCustomNode delegates to the service.
func (e *Executor) IsCustomNode(key string) bool {
	return e.service.IsCustomNode(key)
}

// parseReturnValue converts the goja return value to an ExecResult.
// Expected return: { port: "output", msg: {...} }
// If the script returns nothing, we use the (possibly modified) msg global.
func parseReturnValue(vm *goja.Runtime, val goja.Value, originalMsg map[string]interface{}) (*ExecResult, error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		// Script didn't return anything — use the msg global (which may have been mutated)
		msgVal := vm.Get("msg")
		updatedMsg := exportValue(msgVal)
		if m, ok := updatedMsg.(map[string]interface{}); ok {
			return &ExecResult{Port: "output", Msg: m}, nil
		}
		return &ExecResult{Port: "output", Msg: originalMsg}, nil
	}

	exported := exportValue(val)

	// If return value is a map with "port" and "msg" keys
	if m, ok := exported.(map[string]interface{}); ok {
		port, _ := m["port"].(string)
		if port == "" {
			port = "output"
		}

		msgVal, hasMsgKey := m["msg"]
		if hasMsgKey {
			if msgMap, ok := msgVal.(map[string]interface{}); ok {
				return &ExecResult{Port: port, Msg: msgMap}, nil
			}
		}

		// Return value is a plain object — treat as the msg itself
		return &ExecResult{Port: port, Msg: m}, nil
	}

	return nil, fmt.Errorf("script must return an object with {port, msg} or modify msg directly")
}

// exportValue converts a goja value to a native Go value.
func exportValue(val goja.Value) interface{} {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}

	exported := val.Export()

	// goja returns map[string]interface{} for objects, []interface{} for arrays,
	// but nested objects may still be goja types. Round-trip through JSON to normalize.
	data, err := json.Marshal(exported)
	if err != nil {
		return exported
	}

	var normalized interface{}
	if err := json.Unmarshal(data, &normalized); err != nil {
		return exported
	}

	return normalized
}

// formatArgs converts goja function call arguments to a log-friendly string.
func formatArgs(call goja.FunctionCall) string {
	parts := make([]string, 0, len(call.Arguments))
	for _, arg := range call.Arguments {
		if goja.IsUndefined(arg) {
			parts = append(parts, "undefined")
		} else if goja.IsNull(arg) {
			parts = append(parts, "null")
		} else {
			exported := arg.Export()
			data, err := json.Marshal(exported)
			if err != nil {
				parts = append(parts, fmt.Sprintf("%v", exported))
			} else {
				parts = append(parts, string(data))
			}
		}
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " "
		}
		result += p
	}
	return result
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"fmt"
	"strings"
)

// Routers for the consolidated workflow nodes. Each reads the node's
// selector field and delegates to the executor that backed the former
// stand-alone node. See .references/nodereducing.md for the mapping.

func nodeConfigString(node *CanvasNode, key, fallback string) string {
	v, _ := node.Config[key].(string)
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}

func unsupportedSelector(nodeKey, selector, value string) error {
	return fmt.Errorf("%s: unsupported %s %q", nodeKey, selector, value)
}

func (s *Service) executeParse(node *CanvasNode, msg Msg) (string, Msg, error) {
	switch f := nodeConfigString(node, "format", "json"); f {
	case "json":
		return s.executeParseJSON(node, msg)
	case "yaml":
		return s.executeParseYAML(node, msg)
	case "xml":
		return s.executeParseXML(node, msg)
	case "csv":
		return s.executeParseCSV(node, msg)
	case "html":
		return s.executeParseHTML(node, msg)
	default:
		return "", nil, unsupportedSelector("parse", "format", f)
	}
}

func (s *Service) executeContainerAction(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	switch a := nodeConfigString(node, "action", "start"); a {
	case "start", "stop", "restart", "remove":
		return s.executeDockerAction(ctx, node, msg, envID)
	case "rename":
		return s.executeContainerRename(ctx, node, msg, envID)
	case "wait":
		return s.executeContainerWait(ctx, node, msg, envID)
	default:
		return "", nil, unsupportedSelector("container-action", "action", a)
	}
}

func (s *Service) executeInspect(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	switch r := nodeConfigString(node, "resource", "container"); r {
	case "container":
		return s.executeContainerInspect(ctx, node, msg, envID)
	case "image":
		return s.executeImageInspect(ctx, node, msg, envID)
	case "network":
		return s.executeNetworkInspect(ctx, node, msg, envID)
	case "volume":
		return s.executeVolumeInspect(ctx, node, msg, envID)
	case "stack":
		return s.executeStackStatus(ctx, node, msg, envID)
	default:
		return "", nil, unsupportedSelector("inspect", "resource", r)
	}
}

func (s *Service) executeList(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	switch r := nodeConfigString(node, "resource", "container"); r {
	case "container":
		return s.executeContainerList(ctx, node, msg, envID)
	case "image":
		return s.executeImageList(ctx, node, msg, envID)
	case "network":
		return s.executeNetworkList(ctx, node, msg, envID)
	case "volume":
		return s.executeVolumeList(ctx, node, msg, envID)
	case "stack":
		return s.executeStackList(ctx, node, msg, envID)
	default:
		return "", nil, unsupportedSelector("list", "resource", r)
	}
}

func (s *Service) executeKV(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	switch op := nodeConfigString(node, "operation", "get"); op {
	case "get":
		return s.executeKVGet(ctx, node, msg)
	case "set":
		return s.executeKVSet(ctx, node, msg)
	case "delete":
		return s.executeKVDelete(ctx, node, msg)
	default:
		return "", nil, unsupportedSelector("kv", "operation", op)
	}
}

func (s *Service) executeEnvironmentInfo(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	switch sc := nodeConfigString(node, "scope", "docker"); sc {
	case "docker":
		return s.executeDockerInfo(ctx, node, msg, envID)
	case "host":
		return s.executeHostInfo(ctx, node, msg, envID)
	case "status":
		return s.executeEnvironmentStatus(ctx, node, msg, envID)
	default:
		return "", nil, unsupportedSelector("environment-info", "scope", sc)
	}
}

func (s *Service) executeEncode(node *CanvasNode, msg Msg) (string, Msg, error) {
	alg := nodeConfigString(node, "algorithm", "base64-encode")
	if strings.HasPrefix(alg, "base64-") {
		node.Config["action"] = strings.TrimPrefix(alg, "base64-")
		return s.executeBase64(node, msg)
	}
	return s.executeHash(node, msg)
}

func (s *Service) executeObject(node *CanvasNode, msg Msg) (string, Msg, error) {
	switch op := nodeConfigString(node, "operation", "pick"); op {
	case "pick":
		return s.executePick(node, msg)
	case "omit":
		return s.executeOmit(node, msg)
	case "map":
		return s.executeMap(node, msg)
	default:
		return "", nil, unsupportedSelector("object", "operation", op)
	}
}

func (s *Service) executeFile(node *CanvasNode, msg Msg) (string, Msg, error) {
	switch op := nodeConfigString(node, "operation", "read"); op {
	case "read":
		return s.executeReadFile(node, msg)
	case "write":
		return s.executeWriteFile(node, msg)
	default:
		return "", nil, unsupportedSelector("file", "operation", op)
	}
}

func (s *Service) executeImageManage(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	switch a := nodeConfigString(node, "action", "pull"); a {
	case "pull":
		return s.executeDockerImagePull(ctx, node, msg, envID)
	case "push":
		return s.executeImagePush(ctx, node, msg, envID)
	case "tag":
		return s.executeImageTag(ctx, node, msg, envID)
	case "remove":
		return s.executeImageRemove(ctx, node, msg, envID)
	default:
		return "", nil, unsupportedSelector("image-manage", "action", a)
	}
}

func (s *Service) executeNetworkManage(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	switch a := nodeConfigString(node, "action", "create"); a {
	case "create":
		return s.executeNetworkCreate(ctx, node, msg, envID)
	case "remove":
		return s.executeNetworkRemove(ctx, node, msg, envID)
	case "connect", "disconnect":
		// executeNetworkConnect reads config["action"] itself.
		return s.executeNetworkConnect(ctx, node, msg, envID)
	default:
		return "", nil, unsupportedSelector("network-manage", "action", a)
	}
}

func (s *Service) executeVolumeManage(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	switch a := nodeConfigString(node, "action", "create"); a {
	case "create":
		return s.executeVolumeCreate(ctx, node, msg, envID)
	case "remove":
		return s.executeVolumeRemove(ctx, node, msg, envID)
	case "backup":
		return s.executeVolumeBackup(ctx, node, msg, envID)
	case "restore":
		return s.executeVolumeRestore(ctx, node, msg, envID)
	default:
		return "", nil, unsupportedSelector("volume-manage", "action", a)
	}
}

func (s *Service) executeStackManage(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	switch a := nodeConfigString(node, "action", "deploy"); a {
	case "deploy":
		return s.executeStackDeployRuntime(ctx, node, msg, envID)
	case "up":
		return s.executeComposeUp(ctx, node, msg, envID)
	case "down":
		return s.executeComposeDown(ctx, node, msg, envID)
	case "remove":
		return s.executeStackRemove(ctx, node, msg, envID)
	default:
		return "", nil, unsupportedSelector("stack-manage", "action", a)
	}
}

// isContainerTrigger reports whether node is a container-trigger watching
// the given aspect ("status" or "health"). An unset watch means "status",
// matching the node definition's default.
func isContainerTrigger(node *CanvasNode, watch string) bool {
	if node == nil || node.Action != "container-trigger" {
		return false
	}
	return nodeConfigString(node, "watch", "status") == watch
}

func (s *Service) executeContainerTrigger(ctx context.Context, node *CanvasNode, envID string) (string, Msg, error) {
	if isContainerTrigger(node, "health") {
		return s.executeContainerHealthTrigger(ctx, node, envID)
	}
	return s.executeContainerStatusTrigger(ctx, node, envID)
}

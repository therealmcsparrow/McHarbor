// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

// legacyNodeAlias maps a deprecated node action key to the node that
// absorbed it. The rewrite is applied in memory at execution time (and
// when triggers scan a canvas) so a saved workflow that still references
// an old key keeps running without a database migration. The frontend
// offers the same rewrite through the migration overlay; the two tables
// must agree. See src/frontend/modules/workflows/utils/migrateWorkflow.ts
// and .references/nodereducing.md.
type legacyNodeAlias struct {
	newKey    string
	setConfig map[string]interface{}
	transform func(node *CanvasNode)
}

// legacyNodeAliases is the single source of truth for deprecated keys that map one host
// node onto another. A key a plugin now owns (a per-provider send node, a trigger that
// moved into a plugin) is not listed here: the plugin claims it through a
// workflow-node-alias capability, so the host names no provider (see channel_caps.go).
var legacyNodeAliases = map[string]legacyNodeAlias{
	"cron-trigger":             {newKey: "schedule-trigger", setConfig: nil},
	"parse-json":               {newKey: "parse", setConfig: map[string]interface{}{"format": "json"}},
	"parse-yaml":               {newKey: "parse", setConfig: map[string]interface{}{"format": "yaml"}},
	"parse-xml":                {newKey: "parse", setConfig: map[string]interface{}{"format": "xml"}},
	"parse-csv":                {newKey: "parse", setConfig: map[string]interface{}{"format": "csv"}},
	"parse-html":               {newKey: "parse", setConfig: map[string]interface{}{"format": "html"}},
	"docker-action":            {newKey: "container-action", setConfig: nil},
	"container-power":          {newKey: "container-action", setConfig: nil, transform: transformContainerPower},
	"container-rename":         {newKey: "container-action", setConfig: map[string]interface{}{"action": "rename"}},
	"container-wait":           {newKey: "container-action", setConfig: map[string]interface{}{"action": "wait"}},
	"container-inspect":        {newKey: "inspect", setConfig: map[string]interface{}{"resource": "container"}},
	"image-inspect":            {newKey: "inspect", setConfig: map[string]interface{}{"resource": "image"}},
	"network-inspect":          {newKey: "inspect", setConfig: map[string]interface{}{"resource": "network"}},
	"volume-inspect":           {newKey: "inspect", setConfig: map[string]interface{}{"resource": "volume"}},
	"stack-status":             {newKey: "inspect", setConfig: map[string]interface{}{"resource": "stack"}},
	"container-list":           {newKey: "list", setConfig: map[string]interface{}{"resource": "container"}},
	"image-list":               {newKey: "list", setConfig: map[string]interface{}{"resource": "image"}},
	"network-list":             {newKey: "list", setConfig: map[string]interface{}{"resource": "network"}},
	"volume-list":              {newKey: "list", setConfig: map[string]interface{}{"resource": "volume"}},
	"stack-list":               {newKey: "list", setConfig: map[string]interface{}{"resource": "stack"}},
	"kv-get":                   {newKey: "kv", setConfig: map[string]interface{}{"operation": "get"}},
	"kv-set":                   {newKey: "kv", setConfig: map[string]interface{}{"operation": "set"}},
	"kv-delete":                {newKey: "kv", setConfig: map[string]interface{}{"operation": "delete"}},
	"docker-info":              {newKey: "environment-info", setConfig: map[string]interface{}{"scope": "docker"}},
	"host-info":                {newKey: "environment-info", setConfig: map[string]interface{}{"scope": "host"}},
	"environment-status":       {newKey: "environment-info", setConfig: map[string]interface{}{"scope": "status"}},
	"base64":                   {newKey: "encode", setConfig: nil, transform: transformBase64Algorithm},
	"hash":                     {newKey: "encode", setConfig: map[string]interface{}{"algorithm": "sha256"}},
	"pick":                     {newKey: "object", setConfig: map[string]interface{}{"operation": "pick"}},
	"omit":                     {newKey: "object", setConfig: map[string]interface{}{"operation": "omit"}},
	"map":                      {newKey: "object", setConfig: map[string]interface{}{"operation": "map"}},
	"read-file":                {newKey: "file", setConfig: map[string]interface{}{"operation": "read"}},
	"write-file":               {newKey: "file", setConfig: map[string]interface{}{"operation": "write"}},
	"docker-image-pull":        {newKey: "image-manage", setConfig: map[string]interface{}{"action": "pull"}},
	"image-push":               {newKey: "image-manage", setConfig: map[string]interface{}{"action": "push"}},
	"image-tag":                {newKey: "image-manage", setConfig: map[string]interface{}{"action": "tag"}},
	"image-remove":             {newKey: "image-manage", setConfig: map[string]interface{}{"action": "remove"}},
	"network-create":           {newKey: "network-manage", setConfig: map[string]interface{}{"action": "create"}},
	"network-remove":           {newKey: "network-manage", setConfig: map[string]interface{}{"action": "remove"}},
	"network-connect":          {newKey: "network-manage", setConfig: map[string]interface{}{"action": "connect"}},
	"volume-create":            {newKey: "volume-manage", setConfig: map[string]interface{}{"action": "create"}},
	"volume-remove":            {newKey: "volume-manage", setConfig: map[string]interface{}{"action": "remove"}},
	"volume-backup":            {newKey: "volume-manage", setConfig: map[string]interface{}{"action": "backup"}},
	"volume-restore":           {newKey: "volume-manage", setConfig: map[string]interface{}{"action": "restore"}},
	"stack-deploy":             {newKey: "stack-manage", setConfig: map[string]interface{}{"action": "deploy"}},
	"compose-up":               {newKey: "stack-manage", setConfig: map[string]interface{}{"action": "up"}},
	"compose-down":             {newKey: "stack-manage", setConfig: map[string]interface{}{"action": "down"}},
	"stack-remove":             {newKey: "stack-manage", setConfig: map[string]interface{}{"action": "remove"}},
	"container-status-trigger": {newKey: "container-trigger", setConfig: map[string]interface{}{"watch": "status"}},
	"container-health-trigger": {newKey: "container-trigger", setConfig: map[string]interface{}{"watch": "health"}},
}

// transformContainerPower maps the old container-power action enum
// (enable / disable) onto container-action's enum (start / stop).
func transformContainerPower(node *CanvasNode) {
	switch node.Config["action"] {
	case "enable":
		node.Config["action"] = "start"
	case "disable":
		node.Config["action"] = "stop"
	}
}

// transformBase64Algorithm folds the old base64 node's action
// (encode / decode) into encode's algorithm field.
func transformBase64Algorithm(node *CanvasNode) {
	if _, ok := node.Config["algorithm"]; ok {
		return
	}
	action, _ := node.Config["action"].(string)
	if action != "decode" {
		action = "encode"
	}
	node.Config["algorithm"] = "base64-" + action
	delete(node.Config, "action")
}

// applyLegacyNodeAlias rewrites node.Action and fills the alias's
// setConfig keys that the node does not already carry. Returns true when
// a rewrite happened. Safe to call with any CanvasNode.
func applyLegacyNodeAlias(node *CanvasNode) bool {
	if node == nil {
		return false
	}
	alias, ok := legacyNodeAliases[node.Action]
	if !ok {
		return false
	}
	if node.Config == nil {
		node.Config = map[string]interface{}{}
	}
	if alias.transform != nil {
		alias.transform(node)
	}
	for k, v := range alias.setConfig {
		if _, present := node.Config[k]; !present {
			node.Config[k] = v
		}
	}
	node.Action = alias.newKey
	return true
}

// normalizeLegacyNodes applies applyLegacyNodeAlias to every node of a
// freshly unmarshalled canvas so trigger scans and the executor only ever
// see current action keys.
func normalizeLegacyNodes(canvas *CanvasData) {
	if canvas == nil {
		return
	}
	for i := range canvas.Nodes {
		applyLegacyNodeAlias(&canvas.Nodes[i])
	}
}

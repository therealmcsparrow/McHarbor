// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

// formerSteps is every step key the host catalog shipped, written out literally.
var formerSteps = []formerStep{
	{key: "vm.start", plugin: "addon-kvm", kind: "action", target: "vm", method: "POST", path: "/api/vms/{vm}/start", perms: []string{"vm.manage"}, params: "vm:target:vm"},
	{key: "vm.shutdown", plugin: "addon-kvm", kind: "action", target: "vm", method: "POST", path: "/api/vms/{vm}/shutdown", perms: []string{"vm.manage"}, params: "vm:target:vm,graceSeconds:int:0..3600"},
	{key: "vm.reset", plugin: "addon-kvm", kind: "action", target: "vm", method: "POST", path: "/api/vms/{vm}/reset", perms: []string{"vm.manage"}, params: "vm:target:vm"},
	{key: "vm.force_stop", plugin: "addon-kvm", kind: "action", target: "vm", method: "POST", path: "/api/vms/{vm}/stop", perms: []string{"vm.manage"}, params: "vm:target:vm"},
	{key: "vm.snapshot.create", plugin: "addon-kvm", kind: "action", target: "vm", method: "POST", path: "/api/vms/{vm}/snapshots", perms: []string{"vm.manage"}, params: "vm:target:vm,name:string!,withMemory:bool"},
	{key: "vm.state", plugin: "addon-kvm", kind: "diagnostic", target: "vm", method: "GET", path: "/api/vms/{vm}", perms: []string{"vm.view"}, params: "vm:target:vm"},
	{key: "vm.snapshots", plugin: "addon-kvm", kind: "diagnostic", target: "vm", method: "GET", path: "/api/vms/{vm}/snapshots", perms: []string{"vm.view"}, params: "vm:target:vm"},
	{key: "ct.start", plugin: "addon-lxc", kind: "action", target: "ct", method: "POST", path: "/api/system-containers/{ct}/start", perms: []string{"ct.manage"}, params: "ct:target:ct"},
	{key: "ct.stop", plugin: "addon-lxc", kind: "action", target: "ct", method: "POST", path: "/api/system-containers/{ct}/stop", perms: []string{"ct.manage"}, params: "ct:target:ct"},
	{key: "ct.restart", plugin: "addon-lxc", kind: "action", target: "ct", method: "POST", path: "/api/system-containers/{ct}/restart", perms: []string{"ct.manage"}, params: "ct:target:ct"},
	{key: "ct.freeze", plugin: "addon-lxc", kind: "action", target: "ct", method: "POST", path: "/api/system-containers/{ct}/freeze", perms: []string{"ct.manage"}, params: "ct:target:ct"},
	{key: "ct.unfreeze", plugin: "addon-lxc", kind: "action", target: "ct", method: "POST", path: "/api/system-containers/{ct}/unfreeze", perms: []string{"ct.manage"}, params: "ct:target:ct"},
	{key: "ct.backup.run", plugin: "addon-lxc", kind: "action", target: "ct", method: "POST", path: "/api/system-containers/{ct}/backups", perms: []string{"ct.backup"}, params: "ct:target:ct,kind:enum:full|incremental"},
	{key: "ct.state", plugin: "addon-lxc", kind: "diagnostic", target: "ct", method: "GET", path: "/api/system-containers/{ct}", perms: []string{"ct.view"}, params: "ct:target:ct"},
	{key: "ct.backups", plugin: "addon-lxc", kind: "diagnostic", target: "ct", method: "GET", path: "/api/system-containers/{ct}/backups", perms: []string{"ct.backup"}, params: "ct:target:ct"},
	{key: "compute.capability.probe", plugin: "addon-compute", kind: "action", target: "environment", method: "POST", path: "/api/compute/environments/{$env}/probe", perms: []string{"compute.view"}, params: ""},
	{key: "compute.pool.state", plugin: "addon-compute", kind: "diagnostic", target: "compute_pool", method: "GET", path: "/api/compute/pools", perms: []string{"compute.view"}, params: "pool:target:compute_pool"},
	{key: "compute.capability", plugin: "addon-compute", kind: "diagnostic", target: "environment", method: "GET", path: "/api/compute/environments/{$env}/capability", perms: []string{"compute.view"}, params: ""},
	{key: "swarm.service.scale", plugin: "addon-swarm", kind: "action", target: "swarm_service", method: "POST", path: "/api/swarm/services/{service}/scale", perms: []string{"swarm_services.scale"}, params: "service:target:swarm_service,replicas:int:0..1000!"},
	{key: "swarm.service.restart", plugin: "addon-swarm", kind: "action", target: "swarm_service", method: "POST", path: "/api/swarm/services/{service}/restart", perms: []string{"swarm_services.manage"}, params: "service:target:swarm_service"},
	{key: "swarm.service.state", plugin: "addon-swarm", kind: "diagnostic", target: "swarm_service", method: "GET", path: "/api/swarm/services/{service}", perms: []string{"swarm_services.view"}, params: "service:target:swarm_service"},
	{key: "swarm.service.tasks", plugin: "addon-swarm", kind: "diagnostic", target: "swarm_service", method: "GET", path: "/api/swarm/tasks", perms: []string{"swarm_tasks.view"}, params: "service:target:swarm_service"},
	{key: "swarm.service.logs", plugin: "addon-swarm", kind: "diagnostic", target: "swarm_service", method: "GET", path: "/api/swarm/services/{service}/logs", perms: []string{"swarm_services.view"}, params: "service:target:swarm_service"},
	{key: "swarm.node.state", plugin: "addon-swarm", kind: "diagnostic", target: "swarm_node", method: "GET", path: "/api/swarm/nodes/{node}", perms: []string{"swarm_nodes.view"}, params: "node:target:swarm_node"},
	{key: "k8s.deployment.restart", plugin: "addon-kubernetes", kind: "action", target: "k8s_deployment", method: "POST", path: "/api/kubernetes/deployments/{deployment.namespace}/{deployment.name}/restart", perms: []string{"kubernetes.manage"}, params: "cluster:target:k8s_cluster,deployment:target:k8s_deployment"},
	{key: "k8s.deployment.scale", plugin: "addon-kubernetes", kind: "action", target: "k8s_deployment", method: "POST", path: "/api/kubernetes/deployments/{deployment.namespace}/{deployment.name}/scale", perms: []string{"kubernetes.manage"}, params: "cluster:target:k8s_cluster,deployment:target:k8s_deployment,replicas:int:0..1000!"},
	{key: "k8s.pod.delete", plugin: "addon-kubernetes", kind: "action", target: "k8s_pod", method: "DELETE", path: "/api/kubernetes/pods/{pod.namespace}/{pod.name}", perms: []string{"kubernetes.delete"}, params: "cluster:target:k8s_cluster,pod:target:k8s_pod"},
	{key: "k8s.pod.state", plugin: "addon-kubernetes", kind: "diagnostic", target: "k8s_pod", method: "GET", path: "/api/kubernetes/pods/{pod.namespace}/{pod.name}", perms: []string{"kubernetes.view"}, params: "cluster:target:k8s_cluster,pod:target:k8s_pod"},
	{key: "k8s.pod.logs", plugin: "addon-kubernetes", kind: "diagnostic", target: "k8s_pod", method: "GET", path: "/api/kubernetes/pods/{pod.namespace}/{pod.name}/logs", perms: []string{"kubernetes.logs"}, params: "cluster:target:k8s_cluster,pod:target:k8s_pod"},
	{key: "k8s.deployment.state", plugin: "addon-kubernetes", kind: "diagnostic", target: "k8s_deployment", method: "GET", path: "/api/kubernetes/deployments/{deployment.namespace}/{deployment.name}", perms: []string{"kubernetes.view"}, params: "cluster:target:k8s_cluster,deployment:target:k8s_deployment"},
}

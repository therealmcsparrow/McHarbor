// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export const AgentDockerImage = 'mcharbor-agent:current';

// AgentDefaultTransferListen mirrors the agent's own default for
// MCHARBOR_TRANSFER_LISTEN (src/agent/main.go). Install commands omit the
// variable when it matches, so the operator copies only what they must set.
export const AgentDefaultTransferListen = '0.0.0.0:8788';

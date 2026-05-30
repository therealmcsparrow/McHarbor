// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from "@modules/workflows/types";

export const sendSignal: NodeDefinition = {
  key: "send-signal",
  label: "Send Signal",
  category: "integration",
  description: "Send a message through a Signal API bridge",
  icon: "IconMessageCircle",
  configSchema: [
    {
      key: "communication_mode",
      label: "Delivery",
      type: "select",
      required: true,
      default: "custom",
      options: [
        {
          value: "configured",
          label: "Configured Channel",
        },
        {
          value: "custom",
          label: "Other",
        },
      ],
    },
    {
      key: "channel_id",
      label: "Communication Channel",
      type: "communication-channel-select",
      channelType: "signal",
      required: false,
      showWhen: {
        communication_mode: "configured",
      },
    },
    {
      key: "api_url",
      label: "API URL",
      type: "text",
      required: true,
      showWhen: {
        communication_mode: "custom",
      },
    },
    {
      key: "sender",
      label: "Sender",
      type: "text",
      required: true,
      showWhen: {
        communication_mode: "custom",
      },
    },
    {
      key: "recipient",
      label: "Recipient",
      type: "text",
      required: true,
      showWhen: {
        communication_mode: "custom",
      },
    },
    {
      key: "message",
      label: "Message",
      type: "textarea",
      required: true,
    },
  ],
  inputPorts: ["input"],
  outputPorts: ["output", "error"],
};

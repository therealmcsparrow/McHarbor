// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from "@modules/workflows/types";

export const sendSlack: NodeDefinition = {
  key: "send-slack",
  label: "Send Slack",
  category: "integration",
  description: "Send a message to a Slack webhook",
  icon: "IconBrandSlack",
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
      channelType: "slack",
      required: false,
      showWhen: {
        communication_mode: "configured",
      },
    },
    {
      key: "webhook_url",
      label: "Webhook URL",
      type: "text",
      secret: true,
      required: true,
      showWhen: {
        communication_mode: "custom",
      },
    },
    {
      key: "channel",
      label: "Channel",
      type: "text",
      required: false,
      showWhen: {
        communication_mode: "custom",
      },
    },
    {
      key: "username",
      label: "Username",
      type: "text",
      required: false,
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

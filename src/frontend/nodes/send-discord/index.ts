// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from "@modules/workflows/types";

export const sendDiscord: NodeDefinition = {
  key: "send-discord",
  label: "Send Discord",
  category: "integration",
  description: "Send a message to a Discord webhook",
  icon: "IconBrandDiscord",
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
      channelType: "discord",
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

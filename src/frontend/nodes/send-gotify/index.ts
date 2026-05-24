// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from "@modules/workflows/types";

export const sendGotify: NodeDefinition = {
  key: "send-gotify",
  label: "Send Gotify",
  category: "integration",
  description: "Send a Gotify notification",
  icon: "IconBellRinging",
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
      channelType: "gotify",
      required: false,
      showWhen: {
        communication_mode: "configured",
      },
    },
    {
      key: "server_url",
      label: "Server URL",
      type: "text",
      required: true,
      showWhen: {
        communication_mode: "custom",
      },
    },
    {
      key: "app_token",
      label: "App Token",
      type: "text",
      secret: true,
      required: true,
      showWhen: {
        communication_mode: "custom",
      },
    },
    {
      key: "title",
      label: "Title",
      type: "text",
      required: false,
    },
    {
      key: "message",
      label: "Message",
      type: "textarea",
      required: true,
    },
    {
      key: "priority",
      label: "Priority",
      type: "number",
      required: false,
      default: 5,
      showWhen: {
        communication_mode: "custom",
      },
    },
  ],
  inputPorts: ["input"],
  outputPorts: ["output", "error"],
};

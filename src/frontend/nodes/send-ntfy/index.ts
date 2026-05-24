// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from "@modules/workflows/types";

export const sendNtfy: NodeDefinition = {
  key: "send-ntfy",
  label: "Send ntfy",
  category: "integration",
  description: "Publish a message to an ntfy topic",
  icon: "IconBellPlus",
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
      channelType: "ntfy",
      required: false,
      showWhen: {
        communication_mode: "configured",
      },
    },
    {
      key: "server_url",
      label: "Server URL",
      type: "text",
      required: false,
      default: "https://ntfy.sh",
      showWhen: {
        communication_mode: "custom",
      },
    },
    {
      key: "topic",
      label: "Topic",
      type: "text",
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
      type: "text",
      required: false,
      showWhen: {
        communication_mode: "custom",
      },
    },
    {
      key: "tags",
      label: "Tags",
      type: "text",
      required: false,
      showWhen: {
        communication_mode: "custom",
      },
    },
  ],
  inputPorts: ["input"],
  outputPorts: ["output", "error"],
};

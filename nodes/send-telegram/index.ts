// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from "@modules/workflows/types";

export const sendTelegram: NodeDefinition = {
  key: "send-telegram",
  label: "Send Telegram",
  category: "integration",
  description: "Send a Telegram bot message",
  icon: "IconBrandTelegram",
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
      channelType: "telegram",
      required: false,
      showWhen: {
        communication_mode: "configured",
      },
    },
    {
      key: "bot_token",
      label: "Bot Token",
      type: "text",
      secret: true,
      required: true,
      showWhen: {
        communication_mode: "custom",
      },
    },
    {
      key: "chat_id",
      label: "Chat ID",
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
    {
      key: "parse_mode",
      label: "Parse Mode",
      type: "select",
      required: false,
      showWhen: {
        communication_mode: "custom",
      },
      options: [
        {
          value: "",
          label: "Plain Text",
        },
        {
          value: "Markdown",
          label: "Markdown",
        },
        {
          value: "HTML",
          label: "HTML",
        },
      ],
    },
  ],
  inputPorts: ["input"],
  outputPorts: ["output", "error"],
};

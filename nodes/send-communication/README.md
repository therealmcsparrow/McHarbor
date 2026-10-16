# Send Message

Send a message through a communication channel configured in McHarbor.

## What This Node Does

One node for every communication channel you have added under Settings → Communication. Credentials live there; the node only chooses which channel to use, who receives the message, and what it says.

## When To Use It

Use it wherever a workflow needs to tell a person something: a container went unhealthy, a backup finished, a deploy needs approval. Use the error output to branch into retries or a fallback channel.

## Requirements

At least one enabled channel under Settings → Communication. The picker lists exactly those channels.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the provider rejects the message or cannot be reached.

## Configuration Guide

- `Communication Channel` (select, optional): One of your configured channels. Leave empty to use the default channel.
- `Recipient` (text, optional): Overrides the channel's stored destination for this one send. Leave empty to use the channel default. What it means depends on the provider: an email address, a Telegram chat id, an ntfy topic. Providers whose adapter does not expose a destination reject the send rather than delivering to the stored recipient.
- `Title` (text, optional): Used by channels that support a title.
- `Message` (textarea, required): The message body.

`Recipient`, `Title` and `Message` accept `{{path}}` placeholders resolved against the incoming msg, for example `{{payload.email}}` or `{{payload.container.name}}`. Unknown paths are left as written.

## Execution Notes

The node clones the incoming `msg`, sends the message, and writes the delivery result to `msg.payload` (`notified`, `channelId`, `channelName`, `channelType`, `recipient`, `title`, `message`). A provider error routes to the `error` port with `payload.error` set.

## Migration From The Old Nodes

The former `send-slack`, `send-discord`, `send-teams`, `send-telegram`, `send-whatsapp`, `send-signal`, `send-ntfy` and `send-gotify` nodes are absorbed by this node. Saved workflows that still reference them keep running while the matching comm plugin is installed: each comm plugin claims its own former key through a channel-type `workflow-node-alias` capability, and the backend runs the node as `send-communication` on that plugin's channel type, so an empty channel resolves to the default channel of that provider. The canvas shows it as Send Message. With the plugin uninstalled the node stays on the canvas unresolved and a run passes the message through. The host itself names no provider.

Inline credentials from the old nodes' "Other" delivery mode are no longer used. A migrated node sends through the default configured channel of its type, or the channel it had selected. Add the provider under Settings → Communication if none exists yet.

## Backend Binding

This frontend node is bound to the workflow action key `send-communication` in `src/backend/modules/workflows/service.go`.
Category: `integration`.

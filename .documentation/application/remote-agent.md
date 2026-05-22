# Remote Agent

The McHarbor remote agent allows Docker hosts behind NAT or firewalls to be managed
without exposing the Docker daemon directly.

## Purpose

The agent runs on a remote machine and connects outbound to the McHarbor server over
WebSocket. This makes remote access practical in environments where inbound socket or
TCP access is not acceptable.

## Main File

- `src/agent/agent.go`

## Agent Responsibilities

- build the WebSocket URL from the configured McHarbor URL
- connect to `/api/agent/ws`
- authenticate with the agent token
- report host metadata such as hostname, OS, architecture, and versions
- proxy Docker HTTP traffic through the server transport
- handle exec session traffic for terminal access

## Current Embedded Version

The agent currently reports:

- `1.1.0`

This matters because some backend terminal behavior checks the minimum supported
agent version for interactive exec flows.

## Auth Handshake

On connect, the agent sends:

- token
- hostname
- OS
- architecture
- agent version
- detected Docker version

The server replies with an auth result and environment binding information.

## Transport Model

The agent processes message types such as:

- ping / pong
- HTTP request proxy messages
- request cancellation
- exec start
- exec input
- exec resize
- exec end

## Why the Agent Exists

Without the agent, remote Docker management often depends on:

- exposed TCP Docker daemons
- SSH tunneling
- manually maintained network openings

The agent keeps the control model outbound and centrally managed instead.

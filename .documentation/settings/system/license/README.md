# Licence

**Settings → System → Licence** · `?tab=license`

Your McHarbor licence, what it covers, and how much of it is in use.

## Permissions

| Key | What it unlocks |
|---|---|
| `license.view` | Read the licence state |
| `license.manage` | Change the key |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- The licence key, its tier, its state and its expiry.
- Seats in use, and whether this install currently holds one.

## Nothing is ever switched off

The most important property of McHarbor licensing is stated on every state the screen can
show:

| State | What the banner says |
|---|---|
| **Over licence** | "5 of 3 licensed servers in use." and "**Nothing has been limited — every environment keeps running.**" |
| **Expired** | "**Nothing has been disabled — please renew.**" |
| **Not licensed** | "Nothing has been limited — request a license when it suits you." |
| **Suspended** | "This license is suspended. **Nothing has been disabled.**" |
| **Revoked** | "Nothing has been disabled." A new key is needed |
| **Awaiting approval** | "The key is valid; its term starts once approval comes through." |
| **Not started** | "It becomes active on its start date." |
| **Offline** | "McHarbor has not reached the license server for 9 days. **Licensing is unaffected.**" |

There is no kill switch. An expired, over-used or unreachable licence changes what the
banner says and nothing about what runs.

Dismissing a banner is **per user**: it is recorded against the person who dismissed it, is
audited, and does not clear the banner for anyone else. A banner is shown again to a user only
when a notice is due for them. An API key needs `account.self.manage` to dismiss one.

## Seats

A seat is an **active environment**, however McHarbor reaches it: through an agent, or directly
over a socket, TCP, TLS, SSH or Podman. **Seats in use** is the number of active environments;
a deactivated environment costs nothing. The agent behind an environment is not counted again,
and McHarbor itself is not a seat beyond the environments it manages.

Agents do not claim seats. An agent started with `MCHARBOR_LICENSE_KEY` set logs that the key is
ignored and reports nothing to the licence server. No path in the agent binary is gated on a
licence.

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |

## Related

- [About](../about/README.md): version and dependencies
- [Environments](../../../screens/environments/README.md): each active environment is a seat

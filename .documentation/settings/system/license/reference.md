# Licence: Reference

| Setting | Default | What it does |
|---|---|---|
| **Licence key** | none | The key this install claims against |
| **Licence type** | — | The tier |
| **State** | — | Within licence, over, expired, not licensed, suspended, revoked, awaiting approval, not started |
| **Seats in use** | — | "3 of 5", "3 (unlimited)", or "3 (no license)" |
| **Expires** | — | The date and days remaining, or "No expiry" |
| **Auto-renew** | — | Whether it renews on its own |
| **Last checked in** | — | When the licence server was last reached, or "Never" |
| **Seat** | — | **Held**, or "Not held — activate again to claim one" |

With no licence server configured: "No license server is configured, so this install keeps
the license it has and reports nothing outward." That is a supported configuration for an
air-gapped install. `MCHARBOR_LICENSE_SERVER` sets where the handshake goes.

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| Seats are higher than expected | Every active environment is a seat, agent or direct | Deactivate or remove environments you no longer manage |
| The banner says offline | The licence server has not been reached | "The cached entitlement is being used and nothing has been limited" |
| A feature seems missing after expiry | Licensing gates nothing | Look elsewhere; no path is gated on the licence |
| Seat shows "Not held" | The claim did not complete | Activate again |

## Related

- [Licence](./README.md): what the section is for
- [Settings](../../README.md): the other groups

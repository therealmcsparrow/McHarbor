# Incidents: Behaviour findings

`/incidents/findings`

Some problems never trip an alert: memory that climbs a little every day, a restart every
night that nobody notices, failures that started after Tuesday's deploy. Every hour McHarbor
runs four fixed detectors over the history it already stores and lists what they find.

**The numbers are measured, never generated.** A detector computes them; the page states
them in words. A model may be asked to explain a finding, but the numbers shown stay the
detector's.

| Pattern | Reads | Reported when |
|---|---|---|
| **Memory climbing** | Container memory, the 10-minute rollups of the last 14 days | At least 7 days of data, a straight-line fit of R² ≥ 0.8, and at least 20% growth |
| **Host memory running out** | Host memory percentage, the same rollups | A rising fit of R² ≥ 0.7 that reaches 100% within 30 days |
| **Restarts on a schedule** | Container start and restart events, last 14 days | Starts in the same 20-minute slot of the day (UTC) on at least 3 different days |
| **Failures after a deploy** | Successful GitOps promotions of the last 7 days, and failing exits in the environment | At least 5 failing exits in the day after, and three times as many as in the day before |

A finding that is not seen again for 48 hours is cleared. **Dismiss** hides one until it
clears and is found again. **Explain this finding** needs `incidents.manage` and the
environment's narration opt-in, and records the model and requester like a narrative.

Memory history exists only where McHarbor collects container metrics; see
[Observability](../observability/README.md).

## Related

- [Incidents](./README.md)
- [Narration](./narration.md)

# Runbooks in a post-incident timeline

A post-incident timeline used to answer "what finally fixed it" with a guess. It walked
backwards from the last signal looking for the most recent audit-derived action and relabelled
it *change*. That is a documented heuristic, and it is the best answer available when nobody
recorded what they did.

A [runbook run](../../oncall/runbooks/README.md) is somebody recording what they did. When one
exists, the timeline stops guessing.

## What appears

The timeline gains a `runbook` line beside `symptom`, `event`, `action`, `alert`, `change` and
`resolved`, ordered with the rest by time, and attributed to the person who followed it:

```text
10:00  symptom   restarting repeatedly
10:03  event     die (exit 137)
10:05  runbook   ops — runbook "Restart loop" was abandoned; stopped at step 4
                 (Restart the container), recorded as failed; finding: restart did not
                 clear it; note: escalated to the platform team
10:11  resolved  after 11m0s
```

A run is joined to its incident by `incident_id` when the run was started from the incident,
and by `alert_event_id` otherwise. That is the same key the incident bridge already uses, so no new
join was introduced. At most ten runs are folded into one timeline; more than a handful of
runbooks followed for one incident is itself the finding, and the summary is not the place to
enumerate them.

## Where it stopped is the useful part

The line names the furthest step the operator reached, that step's title, the state they
recorded it in, and their own **finding**. "It stopped at step 4, *restart did not clear it*"
is a fact somebody wrote down during the incident. A backwards audit scan cannot produce it,
and the AI narrator, which only reads the timeline, cannot invent it.

## The fallback stays

`lastChange` is unchanged and still fires for every incident with no run, so nothing about an
existing install's summaries changes until somebody follows a runbook. When a run *is* present
the guess stands down, because a recorded answer and a heuristic guess side by side would
invite the reader to average them.

## No append path, by design

The timeline is a JSON blob on the incident row, written **once**, by `Store.Resolve`, at the
moment the incident is resolved. There is no append.

A runbook run therefore does not write into the timeline as it happens. It is its own rows,
with its own audit trail and its own permission checks, and `BuildTimeline` reads them back at
resolve time, the same way it already reads Docker events, audit rows and alert events. That
is the append path: the run is durable and queryable in its own right, and the blob is
assembled from it rather than mutated during the incident.

The practical consequence for an operator: a run's steps are visible on the on-call runbook
screen and in `mcharbor runbook runs` **while** the incident is open, and in the incident's
timeline **after** it resolves.

## Permissions

Reading a run needs `runbooks.view`. The timeline itself is part of the incident and needs
`incidents.view`; resolving the incident, which is what writes the timeline, needs
`incidents.manage`. Nothing about a runbook run widens either.

## Reporting

A run is lifecycle history, so it is a reporting source: `runbook_runs`, with `runbook`,
`subject`, `responder`, `outcome`, `stopped_at_step`, `stopped_at_title`, `incident`,
`started_at` and `finished_at`. It is consulted by the Incident and problem report and can be
selected in the ad-hoc studio.

The number worth putting in a monthly document is how often a runbook is abandoned at the
*same* step. That is the one that tells an operator their runbook is wrong.

## Related

- [Runbooks](../../oncall/runbooks/README.md): the engine, the step kinds and the permissions
- [Incidents](../README.md): evidence, narration and the rest of the timeline
- [Narration](../narration.md): what the model is and is not allowed to do with a timeline

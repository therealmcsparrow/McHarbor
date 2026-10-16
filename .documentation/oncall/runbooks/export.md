# Exporting a runbook

A runbook leaves McHarbor in three forms, and they answer different questions.

| Format | What it is for | Produced by |
|---|---|---|
| **JSON** | Handing the procedure to another McHarbor install | The host |
| **PDF** | The binder, the audit pack, the printout taped inside the rack door | The Reporting plugin |
| **Word document** | The same thing, when somebody has to edit it outside McHarbor | The Reporting plugin |

The control is the export icon on each row of the Runbooks list, and on each card in the card
view. With only the host's own format reachable it is a single button that downloads the JSON
straight away; with the document formats reachable it opens a menu offering **Export as JSON**,
**Export as PDF** and **Export as Word document**.

## Permissions

| Format | Needs |
|---|---|
| JSON | `runbooks.view` |
| PDF, Word | `runbooks.view` **and** `reporting.generate` |

Exporting is gated on `view`, not `manage`. An export reads, and the people who *follow* a
runbook are the ones who need to hand it to somebody.

The two document formats appear only while the **Reporting** plugin is installed, enabled and
loaded, and only for an operator holding `reporting.generate`. Without either, the control
collapses to the JSON export rather than offering a menu entry that would fail. The host has no
PDF or Word writer of its own, and the plugin's is the only one in the product.

## The JSON file

`GET /api/runbooks/{id}/export` returns the file directly, as
`Content-Type: application/json` with a `Content-Disposition` attachment name. The name is the
runbook's own, lowercased, with every run of non-alphanumeric characters folded to a single dash
and the ends trimmed, then `.mcharbor-runbook.json`, so *Restart the queue worker* downloads as
`restart-the-queue-worker.mcharbor-runbook.json`. A name made only of punctuation would collapse
to nothing, so it falls back to `runbook`; a leading dot would otherwise be a hidden file on
Unix and unopenable on Windows.

The envelope is the same one a [workflow](../../workflows/README.md) export uses, so the two read
as one format family:

```json
{
  "kind": "mcharbor.runbook",
  "version": 1,
  "exportedAt": "2026-09-28T09:14:02Z",
  "runbook": {
    "originalId": "cs7m2q8k9v1e4t0p",
    "name": "Restart the queue worker",
    "description": "When the queue depth alert fires and the worker is wedged.",
    "attachKind": "alert",
    "attachSourceType": "container",
    "attachConditionKey": "container.queue_depth",
    "attachAlertId": "",
    "attachSubject": "",
    "enabled": true,
    "steps": [
      {
        "position": 1,
        "kind": "instruction",
        "title": "Check the dashboard first",
        "body": "If several workers are affected this is not the runbook you want.",
        "required": true
      },
      {
        "position": 2,
        "kind": "diagnostic",
        "title": "Read the container state",
        "diagnosticKey": "container.state",
        "required": false
      },
      {
        "position": 3,
        "kind": "action",
        "title": "Restart it",
        "actionKey": "container.restart",
        "actionParams": {},
        "required": false
      }
    ],
    "automation": {
      "onAlert": false,
      "cron": ""
    },
    "originalCreatedAt": "2026-09-14T11:02:44Z",
    "originalUpdatedAt": "2026-09-27T16:38:10Z"
  }
}
```

`position` is the number the store assigned, kept as it is rather than renumbered from zero,
because a finished run refers to its steps by that number. An empty string or an empty object is
omitted from the file entirely.

## What the file deliberately does not carry

Ids, the environment, the author, and the principal an unattended runbook ran as. Every one of
those is local to the instance that wrote the file: an environment id means nothing in another
install, and a **service principal** in particular is an account on one instance, so a file
carrying it would either collide with something or point at a principal that does not exist.

So `automation` keeps the *schedule* (whether the runbook ran on its alert, and its cron) and
drops the principal. Whoever imports it chooses a principal there.

`originalId`, `originalCreatedAt` and `originalUpdatedAt` are breadcrumbs for the person reading
the file, never keys anything reuses.

There is no import route yet. The file is portable by design and is currently read by people
rather than by McHarbor. There is no `mcharbor runbook export` command either. Exporting is
an interface action for now, and the route is gated on a key the CLI could present, so the
command is queued rather than ruled out.

## The PDF and the Word document

Both are the **Runbooks report** scoped to this one runbook: the runbook as a printable
procedure: what it attaches to, whether it runs unattended and on what schedule, then every step
in order with its body and the diagnostic or action it invokes, and its recent runs. Exporting
therefore generates that report and then renders it, and **the generated report stays in the
report library** where any other report would.

The document never prints a step's action parameters. A parameter can name a secret, and a
printout leaves the permission system behind. See
[Reports](../../reporting/reports/README.md) for the report type itself, including the unscoped
form that says which alert conditions can fire with no runbook behind them.

## Auditing

Every export writes one audit row, `runbook.exported`, with the runbook as the entity and the
operator as the actor. The two document formats add the Reporting plugin's own rows for
generating and exporting a report.

## What goes wrong

| Symptom | Usual cause | What to do |
|---|---|---|
| Only **Export as JSON** is offered | The Reporting plugin is not loaded, or you lack `reporting.generate` | Install or enable it under Plugins; the key is on the Reporting side, not the runbook's |
| The export control is absent entirely | You lack `runbooks.view` | It is the same key the list itself needs, so an empty list is the likelier symptom |
| The PDF takes a few seconds | It is a report being generated, not a file being copied | Expected; the toast confirms when the download starts |
| An imported file's schedule runs as nobody | The file carries no principal, on purpose | Set the principal on the new instance; see [Unattended runs](./unattended.md) |

## Related

- [Runbooks](./README.md): step kinds, authoring, how a step is authorised
- [Unattended runs](./unattended.md): the principal an export never carries
- [Reports](../../reporting/reports/README.md): the `runbooks` report type behind the PDF and
  the Word document
- [Command-line client](../../cli/README.md): `runbook list`, `runbook get`, `runbook runs`

# GitOps: Troubleshooting

When a pipeline does not move. All four causes are configuration rather than failure: a
manual trigger, a branch that does not match the stage, an approval that is still waiting,
or a forge webhook that was never wired up.

| Symptom | Usual cause | Fix |
|---|---|---|
| Nothing deploys on push | The trigger is manual, or the branch does not match the stage | Check the stage’s watch branch against what you pushed |
| A promotion sits at pending | The stage requires approval | Someone with `gitops.approve` has to approve it |
| PR previews never appear | The forge webhook is not configured | Copy the webhook URL and secret from the page into the repository settings |
| A preview environment is left behind | The close event never arrived | Check webhook deliveries on the forge; the preview is demolished on PR close |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [GitOps](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does

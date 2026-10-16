# Stacks: Troubleshooting

What goes wrong with stacks. Most of it follows from one rule: the Compose file is the
source of truth, so a container edited underneath it reverts, a service deleted from it
leaves its container behind, and a stack McHarbor never held a file for cannot be edited
at all.

| Symptom | Usual cause | Fix |
|---|---|---|
| A stack shows as discovered and will not let you edit | McHarbor never held its Compose file | Redeploy it from a file to make it managed, or keep operating it as it is |
| A removed service keeps running | Its container outlived the Compose file | Run **Prune services** |
| Editing a container inside a stack has no lasting effect | The Compose file is the source of truth | Edit the stack, not the container |
| Deploy fails with a parse error | Invalid Compose YAML | The validator names the line; indentation is the usual culprit |
| Webhook test succeeds, real events never arrive | The event list does not include what you expected | Check which of the six events the stack actually fires |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Stacks](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does

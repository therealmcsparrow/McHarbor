# Reconciler: Troubleshooting

When a watch and reality disagree, usually something else is writing to the same
container (an auto-update policy or a GitOps pipeline), or the watch names a container
that has since been renamed.

| Symptom | Usual cause | Fix |
|---|---|---|
| Drift comes back right after reconciling | Something else is changing the container | Check for an auto-update policy or a GitOps pipeline pointed at the same container |
| Drift during every rolling update | Confirmation is too short for this host | Raise **Passes to confirm drift** |
| A watch set to **Enforce** never acts | **Enforcement switched on** is off, enforcement stopped, the watch is in its cooldown, or its plan has a **Needs a person** step | Open **Plans**: every time enforcement does not act, a plan says why it was written; the audit log has a `reconciler.enforce.deferred` entry with the reason |
| "Enforcement stopped after repeated failures" | The breaker opened | Read the reason in the settings, fix the cause, then **Reset and resume** |
| Approving says the drift changed | Reality moved after the plan was written | Review the fresh plan shown in its place |
| Approving says the environment cannot be reached | The agent or host is offline | Nothing was changed; try again when it is back |
| A plan step says **Needs a person** | The change is one McHarbor never makes itself | The step's text says what to do instead |
| A Git source shows **Failing** | Git refused, or a document is invalid | Open the reason behind the icon: it names the file, the document and the problem. No watch changed |
| Editing or deleting a watch is refused | It comes from a Git source | Change or delete its document in the repository |
| **Accept drift in Git** pushed a branch but opened no request | The host is not GitHub, GitLab or Gitea, or the repository has no token | Open the request from the pushed branch yourself |
| A declaration stops matching after a rename | It targets a container name | Update the watch, or keep the container name stable |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. **Enforce** needs `reconciler.enforce`, which someone who
can edit watches may not have. A disabled control with no tooltip is usually state: there
is no drift to propose for, or the watch is a stack or environment, which cannot be
reconciled directly.

## Related

- [Reconciler](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
- [Plans and enforcement](./plans-and-enforcement.md)

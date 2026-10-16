# Reconciler: Plans and enforcement

A plan is the exact change that would make reality match a watch again. **Propose a fix**
writes one for a person to approve; **Enforce** applies it without asking, inside limits
that exist because acting on production on a wrong signal is the worst thing this feature
could do.

## What a plan can do

| Step | When |
|---|---|
| **Start** | A container should run and is stopped |
| **Stop** | A container should be stopped; a container a stack does not declare; a container nobody declared on an environment watch |
| **Remove** | A container should be removed |
| **Recreate** | The image, env, labels, restart policy, memory, CPUs, healthcheck or capabilities differ. Only the drifted values change; everything else the container has is kept. A stored plan, the plan list and its audit entries name only the env keys it changes, each shown as `••••••••`, and only the labels it changes; approving it rebuilds the environment from the running container at that moment, and a recreate that cannot be rebuilt fails rather than write the marker |
| **Deploy stack** | A stack's services are missing or not running |
| **Needs a person** | Anything McHarbor will not do itself, with the reason shown |

Undeclared containers are **stopped, never removed**: the container started by hand during
an incident might be the thing keeping it contained, and a stop can be undone.

**Needs a person** covers: a missing container (a watch does not hold enough to create
one), a container that belongs to a compose stack (change the stack), published ports,
mounts and networks (recreating cannot change them), McHarbor's own container and its agent
(never changed automatically), removing or stopping a whole stack, and a stack whose
compose file comes from Git (its deploy belongs to its pipeline).

## Reviewing and approving

**Plans** lists them, waiting ones first. Opening one shows what drifted and each step.
**Approve and apply** asks once more (**Apply now**), and then McHarbor:

1. checks the environment again. If the drift changed since the plan was written, nothing is
   applied: the plan becomes **Superseded** and the plan for the drift as it is now is shown
   instead, for you to review again;
2. refuses when the environment cannot be reached, rather than guessing;
3. applies the steps in order through the same actions a person's click would use, and stops
   at the first failure. The result of every step is kept on the plan.

A plan can only be applied once, even if two people approve it at the same moment. One plan
waits per watch; a newer drift replaces it, and drift that clears on its own closes it.

## Enforcement

**Enforce** acts only when every one of these holds:

- the watch is set to **Enforce** (needs `reconciler.enforce`);
- **Enforcement switched on** is on in the settings (off by default);
- the drift is confirmed over **Passes to confirm drift**, and the environment could be read;
- every step of the plan can be taken without a person;
- the plan fits in what is left of **Changes per pass** this pass;
- the watch is past its **Cooldown per watch** since enforcement last acted on it.

When any of these fails, a plan is written instead, so there is always something to look at.

**The breaker.** After **Failures before stopping** consecutive failed actions, enforcement
stops everywhere. The settings show when and why; nothing is enforced until someone with
`reconciler.enforce` chooses **Reset and resume**. The **Drift enforcement halted** alert
condition (*Platform* source) is active for as long as it stays stopped.

## Audit

| Event | Severity |
|---|---|
| `reconciler.enforce.enabled` | warning |
| `reconciler.enforce.applied` / `reconciler.enforce.failed` | warning, with the drift, the steps and each step's result |
| `reconciler.enforce.deferred` | info, with why a plan was written instead |
| `reconciler.enforce.breaker_opened` | critical |
| `reconciler.enforce.breaker_reset`, `reconciler.enforcement.updated` | warning |
| `reconciler.proposal.created`, `.applied`, `.failed`, `.rejected`, `.stale` | by outcome |

Enforced actions have no user attached and are recorded as resolved by `system:enforce`, so
"why did this container restart at 4am" has an answer: what was expected, what was found,
and what was done.

## Related

- [Actions](./actions.md): the buttons and what gates them
- [Git sources](./git-sources.md): intent from a repository
- [Troubleshooting](./troubleshooting.md)

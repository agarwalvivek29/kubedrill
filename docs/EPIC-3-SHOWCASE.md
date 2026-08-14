# Epic 3 — the integrity layer, proven

**TL;DR:** kubedrill challenges can now carry **rules** graded exam-style from the
API-server **audit log** — *nothing blocks you, but you lose points for breaking
the rules* — with optional **live enforcement** at admission and **node-level**
drills. To show it end-to-end we authored three integrity challenges through the
toolchain, proved each with `author test` on real clusters, and then played the
role of a cheating player to watch the rules actually fire.

## What Epic 3 delivers

The pipeline: **capture → attribute → grade → enforce → node-level.**

| Capability | What it does |
|------------|--------------|
| **Two-tier audit capture** | The apiserver records mutating actions (secrets Metadata-only; field-`require` kinds at Request). Streamed incrementally off the node. |
| **AD-4 attribution** | Only *your* actions count. The engine and pinned controllers are exempt; a player-created ServiceAccount or an impersonation is charged to you — the loophole is closed by construction. |
| **Grade deny/protect/require** | `deny` = you did a forbidden thing; `protect` = you deleted a protected object; `require` = you didn't do a required thing. Each violation carries the audit event as evidence. |
| **Live enforcement** | `enforce: true` additionally installs a ValidatingAdmissionPolicy that blocks the action at admission — exempting *exactly* the same AD-4 set (never `system:*`). |
| **Node-level + advisory** | `nodeAccess` grants `kubedrill node-shell <node>`; because root-on-node can defeat audit tamper-evidence, rules there run **advisory** and the scorecard says so. |

## The demos

Three challenges, each authored via `author new → validate → lint → test`:

| Challenge | Shows | Rules |
|-----------|-------|-------|
| `payments-hotfix` | **graded** rules | `protect` the Deployment (fix in place, `fail`), `deny` touching kube-system (−25) |
| `guarded-config` | **live enforcement** | `protect … enforce: true` on a ConfigMap — deletion blocked at admission |
| `node-recon` | **node access + advisory** | root shell on the node; `protect` rule graded advisory |

All three are `validate`-clean, `lint`-clean, and **pass `author test`** (negative → positive → idempotency) on real kind — so the reference solutions are proven rule-compliant, and the CI content gate runs the same check on every change.

```
$ kubedrill author test challenges/payments-hotfix
  ✓ negative     all 1 objective(s) behaved correctly on the fresh environment
  ✓ positive     100% — 100/100 points across 1 objective(s)
  ✓ idempotency  100% — 100/100 points across 1 objective(s)
✓ payments-hotfix is solvable and non-vacuous

# guarded-config and node-recon likewise: all three ✓ solvable and non-vacuous.
```

## Playing the cheat: the rules firing

The reference solutions comply with the rules, so `author test` stays green. To
show the *other* path, we started each challenge and misbehaved.

### `payments-hotfix` — graded violations with evidence

The cheat: delete-and-recreate `payments` (which "fixes" it — but that's a shortcut) and poke a `kube-system` Deployment. The objective passes, but the audit log charges the shortcuts to the player:

```
$ kubedrill verify
  ✓ [100 pts] payments has 3 available replicas

Rule violations:
  ✗ [protect] fix-in-place (FAIL)  — deleted the protected Deployment prod/payments
      • delete deployments prod/payments (as kubernetes-admin)
  ✗ [deny] hands-off-system (−25)  — performed a denied action on Deployment in namespace kube-system
      • create deployments kube-system/coredns (as kubernetes-admin)
      • patch  deployments kube-system/coredns (as kubernetes-admin)

Objectives: 100/100   Rule penalty: −25
Score: 0/100
Challenge failed: an integrity/fail rule was violated. 🚫
```

Objective met, run **failed** — with the exact audit events as evidence, and *only* the player's own actions charged (the controllers and engine that also touch `kube-system` are never blamed).

### `guarded-config` — blocked live at admission

The `warehouse` ConfigMap is protected with `enforce: true`. The player never even gets the chance to lose points — the delete is rejected at admission:

```
$ kubectl -n analytics delete configmap warehouse
Error from server (Forbidden): configmaps "warehouse" is forbidden:
  ValidatingAdmissionPolicy 'kubedrill-enforce-protect-warehouse-config'
  denied request: kubedrill: the warehouse ConfigMap is protected — deleting it is blocked

$ kubectl -n analytics get configmap warehouse -o name
configmap/warehouse          # still there
```

The engine that sets up and resets the challenge is exempt from the same policy (it shares *exactly* the AD-4 exempt set), so the environment is managed normally — only the player's forbidden action is blocked. `start` waits for the guardrail to be live before handing over the cluster, so there's no window to slip through.

### `node-recon` — node shell + advisory scorecard

`node-recon` grants a root shell on the node, where the fault dropped an incident note:

```
$ kubedrill node-shell control-plane
root@…-control-plane:/# cat /root/INCIDENT.txt
INCIDENT (on-call notes):
The web container command was overridden to exit 1 during a bad deploy.
Restore it with: kubectl -n edge patch deployment web --type=json -p ...(remove the command)
```

Because root-on-node can defeat audit tamper-evidence, rules here are **advisory** — the same delete-and-recreate that *failed* `payments-hotfix` is only informational:

```
$ kubedrill verify
  ✓ [100 pts] web has 2 available replicas

Rule violations (advisory — node access; informational only):
  ✗ [protect] fix-in-place (FAIL)  — deleted the protected Deployment edge/web
      • delete deployments edge/web (as kubernetes-admin)

Objectives: 100/100
Score: 100/100
All objectives passed. 🎉
```

kubedrill shows the violation for learning but **doesn't pretend** the grade is tamper-proof — it says so on the scorecard and leaves the score intact.

## Status

**Epic 3 is complete (5/5).** The full integrity layer ships and is proven on
real clusters; built-in content grew to 9 challenges. Next is **Epic 4 — share
it and ship it** (starter library, pack sharing + score comparison, and a
signed single binary via goreleaser).

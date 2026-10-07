# hyp: hypothesis tree CLI

This directory is a hyp research directory. A human is testing hypotheses by breaking each one
down into sub-hypotheses. Your main job is to help decompose hypotheses. The human checks facts
and approves evidence.

## Rules

- Change `hyp.yaml`, `hypotheses/` and `evidence/` only through `hyp` commands. Never edit,
  write, move or delete these files directly. Every `hyp` command commits its own changes;
  a direct edit is reported as an error by `hyp check`.
- Never run `hyp evidence approve` or `hyp undo --force`. Approval means a human has checked
  the evidence.
- You may add evidence candidates you found (`hyp evidence add`). They stay unapproved until a
  human checks them. Never present unapproved evidence as fact.
- To read the tree, use `--json` (`hyp show <id> --json`, `hyp conclude <id> --json`,
  `hyp evidence show <id> --json`) instead of parsing the human-readable output.
- If you made a mistake with your last command, run `hyp undo`.

## Data model

- Each hypothesis has a random 8-hex-digit id. Any unique prefix works as an argument.
  Labels like `A-1-2` are computed for display and change when the tree changes; do not store them.
- Only leaves have a `status`: untested, testing, supported, rejected, on-hold.
  A hypothesis with children has no status; its conclusion is derived by `hyp conclude`.
  Adding a child to a leaf clears its status.
- A hypothesis with children has `combine`, which says how the children make it up:
  - `and`: it holds if all children hold
  - `or`: it holds if any child holds
  - `cases`: the children split it into cases. Each child has a `condition`, and the
    conclusion becomes conditional ("supported when ...").
  To express `(A and B) or C`, put an `and` hypothesis holding A and B under an `or` hypothesis.
  If you cannot give that intermediate hypothesis a meaningful title, reconsider the decomposition.
- Evidence is shared: one piece of evidence can be linked to several hypotheses. Each link has
  a `stance`: supports, refutes or inconclusive.

## Commands

    hyp show [id] [--json]                  list roots, or show the tree under a hypothesis
    hyp add [parent] <title>                add a hypothesis (prints its id)
    hyp set <id> [--title ...] [--description ...] [--method ...]
                 [--status ...] [--combine ...] [--condition ...]
    hyp mv <id> (<new-parent> | --root)
    hyp rm <id>                             only hypotheses without children
    hyp conclude <id> [--json]              derived conclusion and the leaves still pending
    hyp evidence add <hypothesis> --summary <text> --stance <stance>
                 [--file <path>]... [--source <url>]
    hyp evidence link <hypothesis> <evidence> --stance <stance>
    hyp evidence unlink <hypothesis> <evidence>
    hyp evidence show <evidence> [--json]
    hyp evidence rm <evidence>              only evidence not linked from any hypothesis
    hyp check [--json]                      integrity check
    hyp undo                                undo the last hyp commit

## Decomposing a hypothesis

1. Read the current tree with `hyp show <id> --json`.
2. Propose sub-hypotheses to the human before adding them: say which `combine` you would use
   and why the children are mutually exclusive and collectively exhaustive.
3. After agreement, add them with `hyp add <parent> <title>`, set `--combine` on the parent,
   and set `--condition` on each case if the parent is `cases`.
4. For each leaf, suggest a `--method`: how it can be tested and what result would reject it.
5. Look for ways the hypothesis could be wrong, not only for support.
6. Run `hyp check` and fix what you caused.

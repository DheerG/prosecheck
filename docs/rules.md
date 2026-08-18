# Understand rules and results

prosecheck applies local rules before it runs an optional model review.

Each rule accepts `error`, `warning`, `info`, or `off`. Strict checks treat warnings as failures.

## Local rules

| Rule | Meaning | Default |
|---|---|---|
| `PC001` | Subject is empty | Error |
| `PC002` | Subject exceeds the character limit | Warning |
| `PC003` | Subject gives too little information | Warning |
| `PC004` | Subject ends with a period | Warning |
| `PC005` | Subject is a temporary label | Error |
| `PC006` | Subject names work but not its result | Warning |
| `PC007` | Subject starts in the past tense | Warning |
| `PC008` | Blank line is missing after the subject | Warning |
| `PC009` | Body starts with implementation details | Warning |
| `PC010` | Body narrates the commit process | Warning |
| `PC011` | Body line exceeds the character limit | Warning |
| `PC012` | Body sentence exceeds the word limit | Warning |
| `PC013` | Body repeats the subject | Information |
| `PC014` | Body contains a semicolon | Warning with Simple English |
| `PC015` | Text contains a contraction | Warning |
| `PC016` | Text contains a wordy or indirect phrase | Warning |
| `PC017` | A modal verb makes the meaning uncertain | Warning |
| `PC018` | Text contains a Latin abbreviation | Warning |
| `PC019` | Text uses a complex verb tense | Warning |

Pragmatic Simple English reports `PC017` and `PC019` as information unless the configuration changes their severity.

Merge commits, revert commits, and autosquash subjects do not use these rules.

## Model findings

The optional model reports `SEM001` through `SEM006`. These findings cover missing context, missing reasons, unclear outcomes, shorthand, diff conflicts, and vague subjects.

Model findings are information in this version. They do not block a commit.

## Exit codes

| Code | Meaning |
|---:|---|
| `0` | The message passed the active policy. |
| `1` | The message failed the active policy. |
| `2` | prosecheck could not complete the check. |

Use JSON output when another program reads the result:

```sh
prosecheck check --format json .git/COMMIT_EDITMSG
```

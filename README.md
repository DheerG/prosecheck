# prosecheck

prosecheck finds unclear Git commit messages before they enter the project history.

The command uses fast local rules by default. An optional local model can find missing context and vague language.

The Git hook does not need GitHub Actions. It runs on each local computer and does not use an external API.

## Current scope

This first version checks commit subjects and bodies. Support for pull request text will use the same checker in a later version.

prosecheck finds these common problems:

- Empty or temporary subjects, such as `WIP`
- Vague subjects, such as `Update code`
- Past-tense subjects, such as `Updated account checks`
- Missing blank lines after the subject
- Bodies that start with file lists or implementation details
- Bodies that narrate the commit process
- Long lines and long sentences
- Contractions, wordy phrases, and uncertain modal verbs
- Context that will not make sense later

## Build prosecheck

Go 1.22 or a later compatible release is required.

```sh
go test ./...
go install ./cmd/prosecheck
```

Make sure that the Go binary directory is on `PATH`. Then run this command:

```sh
prosecheck version
```

You can also install the latest public version:

```sh
go install github.com/DheerG/prosecheck/cmd/prosecheck@latest
```

## Set up a repository

Run the guided setup from any directory inside the Git repository:

```sh
prosecheck init
```

The setup explains the local rules, Git hook, and optional model. It asks before it writes files or downloads the model.

Use the recommended choices without questions:

```sh
prosecheck init --yes
```

This choice enables pragmatic Simple English and installs the Git hook. It does not install the optional model.

Enable the model during an automated setup:

```sh
prosecheck init --yes --semantic on
```

Use `prosecheck init --advanced` to answer every feature question. The command never stages the new configuration in Git.

Select strict Simple English without questions:

```sh
prosecheck init --yes --simple-english strict
```

## Check a message

Pass a message directly:

```sh
prosecheck check --message "Prevent duplicate invoice delivery"
```

Pass a message file:

```sh
prosecheck check .git/COMMIT_EDITMSG
```

Pass a message through standard input:

```sh
git log -1 --format=%B | prosecheck check
```

Warnings do not cause an error by default. Add `--strict` to return an error for warnings.

```sh
prosecheck check --strict --message "Updated code"
```

If another program reads the result, use `--format json`.

## Install only the Git hook

Use `prosecheck init` for a first setup. Use `install-hook` when the repository already has a configuration.

The hook calls `prosecheck` from `PATH`. Run this command in the repository:

```sh
prosecheck install-hook
```

The hook uses strict mode for local rules. It blocks a commit when a rule reports an error or warning.

The installer uses these rules:

| Repository setup | Installer action |
|---|---|
| Plain Git | Create a managed `commit-msg` hook. |
| Custom Git hooks path | Create the hook in the configured path. |
| Husky | Add a managed block to `.husky/commit-msg`. |
| Lefthook | Show the entry to add to the Lefthook configuration. |
| pre-commit | Show the local hook entry and installation command. |
| Overcommit | Show the entry to add to `.overcommit.yml`. |

The installer never replaces an existing custom hook. It also never changes Husky files in `.husky/_`.

Remove a hook that prosecheck manages:

```sh
prosecheck uninstall-hook
```

Read [Connect prosecheck to Git hooks](docs/hooks.md) for manager-specific examples.

## Configuration

The guided setup creates `.prosecheck.json`. You can also copy the example file into the project root:

```sh
cp .prosecheck.example.json .prosecheck.json
```

prosecheck searches for `.prosecheck.json` from the Git project root. Use `--config` to select a different file.

Each rule can use `error`, `warning`, `info`, or `off`.

```json
{
  "rules": {
    "PC017": "warning",
    "PC005": "error"
  }
}
```

The file merges with the default configuration. You only need to include values that you want to change.

### Simple English

Simple English is enabled by default in pragmatic mode. Its local rules find contractions, indirect phrases, modal verbs, and complex tenses.

Pragmatic mode blocks high-confidence findings. It reports modal verbs and complex tenses as notes.

Strict mode applies the configured severity to every Simple English rule.

```json
{
  "simpleEnglish": {
    "enabled": true,
    "mode": "pragmatic",
    "severity": "warning",
    "allow": ["OAuth", "SAML", "webhook"]
  }
}
```

The `allow` list protects project terms that overlap a language rule. Code in backticks and quoted errors are also protected.

The profile uses practical rules from Simplified Technical English. It does not claim full ASD-STE100 compliance.

Read [Use Simple English](docs/simple-english.md) for the complete policy and examples.

## Add a local model review

The local model review is optional. Local rules work without a model.

The reviewer checks for these problems:

- Context that will disappear
- Missing reasons
- Important outcomes hidden by details
- Local jargon or shorthand
- Differences between the message and the staged diff
- Subjects that do not identify the outcome

Model findings are notes in this version. They do not block a commit, even when the hook uses strict mode.

The guided setup can install the model. You can also install it directly:

```sh
prosecheck model install bonsai-8b
```

This command downloads a pinned Prism runtime and a 1.16 GB model file. It verifies both files before installation.

Enable the reviewer in `.prosecheck.json`:

```json
{
  "semantic": {
    "enabled": true,
    "runtime": "managed",
    "model": "bonsai-8b",
    "timeout": "20s",
    "maxDiffBytes": 12000
  }
}
```

prosecheck starts the model when a check needs it. The server listens only on the local computer.

The preferred port is `11435`. If that port is busy, prosecheck selects a free port and records it outside the repository.

Use these commands to inspect or control the model:

```sh
prosecheck model status
prosecheck model doctor
prosecheck model logs
prosecheck model stop
```

The managed runtime does not use Ollama.

Use `--semantic on` for one required model review. The command returns an operational error when the model cannot respond.

```sh
prosecheck check --semantic on --message "Explain the durable outcome"
```

Use `--semantic off` to skip a reviewer that the configuration enables.

Read [Run Bonsai locally](docs/bonsai.md) for storage details, file imports, and an external-server setup.

Read [Compare Bonsai 4B and 8B](docs/model-comparison.md) for raw benchmarks and the synthetic semantic-review eval.

## Exit codes

| Code | Meaning |
|---:|---|
| `0` | The message passed the active policy. |
| `1` | The message failed the active policy. |
| `2` | prosecheck did not complete the check. |

## Rule reference

| Rule | Meaning | Default |
|---|---|---|
| `PC001` | Empty subject | Error |
| `PC002` | Subject exceeds the character limit | Warning |
| `PC003` | Subject gives too little information | Warning |
| `PC004` | Subject ends with a period | Warning |
| `PC005` | Subject is a temporary label | Error |
| `PC006` | Subject names work but not its outcome | Warning |
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
| `PC017` | A modal verb makes the meaning uncertain | Note in pragmatic mode |
| `PC018` | Text contains a Latin abbreviation | Warning |
| `PC019` | Text uses a complex verb tense | Note in pragmatic mode |
| `SEM001`–`SEM006` | Optional model findings | Information |

Merge commits, revert commits, and autosquash subjects do not use these rules.

## Development

Run all automated checks:

```sh
gofmt -w cmd internal
go test ./...
go vet ./...
```

The project uses only the Go standard library. Tests do not require a model download.

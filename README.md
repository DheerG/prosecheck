# prosecheck

prosecheck keeps agent-written Git history useful.

Coding agents often describe changed files or recent actions. These messages lose value when the task ends.

Message standards can also drift between agents, tools, and sessions. prosecheck turns those standards into one shared policy.

prosecheck checks each commit message before Git records it. It helps people and agents understand what changed, why it changed, and which decisions still matter.

```text
Update auth files
```

becomes:

```text
Prevent expired sessions from reaching account pages

Reject the session before route handlers run. This keeps the access rule consistent across account pages.
```

The check runs on the local computer. It does not require a hosted service or spend GitHub Actions minutes.

## Install

### macOS or Linux

```sh
curl -fsSL https://raw.githubusercontent.com/DheerG/prosecheck/main/install.sh | sh
```

### Windows PowerShell

```powershell
irm https://raw.githubusercontent.com/DheerG/prosecheck/main/install.ps1 | iex
```

The installers download the correct binary from the latest GitHub release. They verify its SHA-256 checksum and do not require Go.

You can also download an archive from the [GitHub releases page](https://github.com/DheerG/prosecheck/releases).

## Start

Open a Git repository and run:

```sh
prosecheck init
git add .prosecheck.json
```

The guided setup creates a shared policy and connects the local Git hook. It explains each choice before it changes the repository.

Use the recommended setup without questions:

```sh
prosecheck init --yes
```

The recommended setup uses strict Simple English and blocks commits with warnings. It does not install the optional model.

## What prosecheck catches

prosecheck finds common sources of weak history:

- Temporary or vague subjects, such as `WIP` or `Update code`
- Subjects that describe activity instead of the result
- Bodies that list files but omit the reason for the change
- Implementation detail that hides the effect on the system
- Context that will not make sense after the current task
- Long, indirect, or uncertain language

Merge commits, reverts, and autosquash commits keep their standard formats.

## How it works

Fast local rules perform every check. They produce the same result without a network connection or external API.

An optional local model can review meaning. It finds missing reasons, local shorthand, and differences between the message and the staged change.

Model findings are notes in this version. They do not block a commit. The model runs on your computer and does not use Ollama.

This design gives each repository one durable policy. Any person or agent that uses Git receives the same feedback.

## Common commands

Check text directly:

```sh
prosecheck check --message "Prevent duplicate invoice delivery"
```

Check a message file:

```sh
prosecheck check .git/COMMIT_EDITMSG
```

Install the hook when a policy already exists:

```sh
prosecheck install-hook
```

Allow warnings for one manual check:

```sh
prosecheck check --strict=false --message "Updated account checks"
```

Bypass one commit only when you cannot correct its message:

```sh
PROSECHECK_BYPASS=1 git commit
```

prosecheck prints a notice when it skips the check.

## Configuration

`prosecheck init` writes `.prosecheck.json` in the repository root. Commit this file so people and agents share the same policy.

You only need to record values that differ from the defaults:

```json
{
  "rules": {
    "PC017": "info",
    "PC005": "error"
  }
}
```

Each rule accepts `error`, `warning`, `info`, or `off`.

### Simple English

Simple English is enabled in strict mode by default. It finds contractions, indirect phrases, modal verbs, and complex verb tenses.

Pragmatic mode reports modal verbs and complex tenses as notes. It still blocks high-confidence language problems.

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

The profile uses practical rules from Simplified Technical English. It does not claim full ASD-STE100 compliance.

Read [Use Simple English](docs/simple-english.md) for the policy and examples.

### Private model review

Enable the model during setup:

```sh
prosecheck init --yes --semantic on
```

Or install it separately:

```sh
prosecheck model install ministral-3-8b
```

The command downloads a pinned runtime and a 5.20 GB model. It shows progress and stores the model in the standard Hugging Face cache.

Useful model commands:

```sh
prosecheck model status
prosecheck model doctor
prosecheck model logs
prosecheck model stop
```

Read [Run the local model](docs/local-model.md) for storage, privacy, and external-server settings.

Read [Compare local models](docs/model-comparison.md) for benchmark and synthetic evaluation results.

## Reference

- [Rules and exit codes](docs/rules.md)
- [Git hooks and hook managers](docs/hooks.md)
- [Simple English policy](docs/simple-english.md)
- [Local model](docs/local-model.md)
- [Model comparison](docs/model-comparison.md)

Commit messages are the current focus. A later version can apply the same policy to pull request titles and descriptions.

## Development

Go 1.22 or a later compatible release is required for source builds.

```sh
go test ./...
go install ./cmd/prosecheck
```

You can also install the current source version:

```sh
go install github.com/DheerG/prosecheck/cmd/prosecheck@latest
```

The project uses only the Go standard library. Tests do not require a model download.

Maintainers can read [Create a release](docs/releases.md) for the tag and package process.

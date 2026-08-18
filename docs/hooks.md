# Connect prosecheck to Git hooks

Install the `prosecheck` executable once on each computer. Do not copy the executable into each repository.

Each repository needs these two items:

- A `.prosecheck.json` policy file
- A `commit-msg` hook that calls prosecheck

The repository files let a team share one policy. The local hook does not use GitHub Actions.

The check treats warnings as failures. To bypass one commit, set `PROSECHECK_BYPASS=1` for that command:

```sh
PROSECHECK_BYPASS=1 git commit
```

Prosecheck prints a notice when it skips the check.

## Set up a repository

Run this command from the repository root:

```sh
prosecheck init
```

The guided setup creates the policy file and connects the hook. It also offers the optional private model.

Use this command when the policy file already exists:

```sh
prosecheck install-hook
```

Both commands detect plain Git, Husky, Lefthook, pre-commit, and Overcommit.

## Plain Git

If no hook manager exists, the installer creates the effective `commit-msg` hook. This also supports a custom `core.hooksPath` value.

The installer stops if a custom `commit-msg` hook already exists. Add this command to the existing hook:

```sh
prosecheck check --strict "$1"
```

## Husky

The installer adds a managed block to `.husky/commit-msg`. It keeps all existing commands in that file.

For example, this command can remain before the managed block:

```sh
commitlint --edit "$1"
```

The installer does not change `.husky/_`. Husky generates the files in that directory.

Run `prosecheck uninstall-hook` to remove only the managed block. The command keeps the other Husky commands.

## Lefthook

Add this command to the `commit-msg` section of the Lefthook configuration:

```yaml
commit-msg:
  commands:
    prosecheck:
      run: prosecheck check --strict "{1}"
```

Then install the Lefthook hooks:

```sh
lefthook install
```

## pre-commit

Add this local hook to `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: local
    hooks:
      - id: prosecheck
        name: prosecheck
        entry: prosecheck check --strict
        language: system
        stages: [commit-msg]
```

If the file already has a `repos` list, add only the local repository entry.

Then install the `commit-msg` hook:

```sh
pre-commit install --hook-type commit-msg
```

## Overcommit

Add this entry under `CommitMsg` in `.overcommit.yml`:

```yaml
CommitMsg:
  Prosecheck:
    enabled: true
    required_executable: prosecheck
    command: [prosecheck, check, --strict]
```

Then install the Overcommit hooks. Then sign the configuration:

```sh
overcommit --install
overcommit --sign
```

## Remove the hook

For plain Git and Husky, run this command:

```sh
prosecheck uninstall-hook
```

For another hook manager, remove the prosecheck entry from its configuration. Then run the installation command for that manager again.

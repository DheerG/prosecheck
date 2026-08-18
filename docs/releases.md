# Create a release

The release process starts when a maintainer pushes a version tag. Normal commits and pull requests do not start this workflow.

## Publish a version

First, confirm that `main` contains the intended changes. Then create and push a Semantic Version tag:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The release workflow performs these tasks:

1. Run the Go tests and static checks.
2. Build binaries for macOS, Linux, and Windows.
3. Package Intel and Arm binaries for each system.
4. Create SHA-256 checksums.
5. Create a GitHub release with generated notes.

A tag with a suffix, such as `v0.2.0-rc.1`, creates a prerelease.

## Test packages locally

Use a temporary output directory so the repository stays clean:

```sh
PROSECHECK_DIST_DIR=/tmp/prosecheck-dist ./scripts/package-release.sh v0.1.0
```

The script requires Go, `tar`, `zip`, and a SHA-256 command.

## Future pull request labels

The release note configuration already recognizes these labels:

| Label | Release note group |
|---|---|
| `version:major` | Breaking changes |
| `version:minor` or `enhancement` | Features |
| `version:patch` or `bug` | Fixes |
| `skip-changelog` | Omitted from release notes |

These labels do not change versions yet. A later pull request workflow can use them to select the next version tag.

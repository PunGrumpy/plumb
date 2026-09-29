<!-- contentType: How-to · plan: docs/content-plan.md -->

# Release plumb

This page gives the steps from recording a change to getting binaries on a GitHub Release. plumb uses Changesets to manage the version and `apps/cli/CHANGELOG.md`, and GoReleaser to build binaries. Both run in `.github/workflows/release.yml`.

plumb's version is in `apps/cli/package.json`. This file is `private` and exists only for Changesets. plumb itself doesn't use Node.

## Add a changeset to your PR

Every PR that users should know about needs 1 changeset file.

1. The first time on your machine, install Changesets with [Bun](https://bun.sh):

   ```sh
   bun install
   ```

2. Create a changeset and pick the level of the change:

   ```sh
   bun changeset
   ```

3. Write a summary for users. This text goes into `CHANGELOG.md` and the release notes exactly as you write it.
4. Commit the new file in `.changeset/` along with your code.

Pick the level from this table:

| Level | Use it when | Before 1.0.0 |
| --- | --- | --- |
| `patch` | You fix a bug or text without adding features | 0.1.0 to 0.1.1 |
| `minor` | You add a command, flag or check | 0.1.0 to 0.2.0 |
| `major` | You remove or change a command, flag or JSON field that others use | 0.1.0 to 1.0.0 |

PRs that users don't see, such as test or CI changes, don't need a changeset.

## Cut a release

1. Merge PRs with changesets into `main`.
2. The workflow opens or updates a PR titled "chore: version packages", which bumps the version in `apps/cli/package.json`, writes `apps/cli/CHANGELOG.md` and deletes the used changeset files.
3. Check `apps/cli/CHANGELOG.md` in that PR. If you want to change the text, edit it in the PR.
4. Merge the "chore: version packages" PR when you're ready to release.

The "chore: version packages" PR collects every changeset merged up to that point. If you don't want to release yet, leave that PR open.

After the merge, the workflow creates the tag `vx.y.z` from the version in `apps/cli/package.json`. GoReleaser then builds binaries for Linux and macOS on both `amd64` and `arm64`, creates a GitHub Release that uses that version's section of `apps/cli/CHANGELOG.md` as the release notes, and attaches the `.tar.gz` files and `checksums.txt`.

## What a release puts in the binary

GoReleaser sets these values at build time through `-ldflags`:

- `main.version` is the tag name, such as `v0.2.0`, which `plumb version` shows.
- `main.updateURL` is `https://api.github.com/repos/<owner>/<repo>/releases/latest` for the repo that builds it, so the binary tells you when a new release is out.

If you build with `make -C apps/cli dist` without setting `UPDATE_URL`, the binary doesn't check for new versions.

## Check the config before you push

If you change `.goreleaser.yaml`, check it and try a build on your machine first:

```sh
goreleaser check
goreleaser build --snapshot --clean --single-target
```

If you change files in `.github/workflows/`, check them with `actionlint`. If you change `.changeset/config.json`, see the result with `bun changeset status`.

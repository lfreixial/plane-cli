# Releasing Plane CLI

Releases are GitHub Releases containing the CLI binaries. No Plane server or
other hosted service is deployed by this repository.

## Workflows

- **CI** runs for pull requests and pushes to `main`. It checks formatting,
  dependency consistency, vet, race tests, executable startup, shell completion,
  workflow syntax, and GoReleaser configuration. Tests run on Linux (the minimum
  Go version and current stable Go), macOS, and Windows.
- **Security** scans dependencies for all six release targets during CI and
  weekly. License collection also runs in CI and before packaging releases.
- **Release** runs when a version tag is pushed. It reuses CI to validate the
  tagged commit, then builds and publishes the archives and checksums. Only the
  publishing job receives `contents: write`; it uses GitHub's automatic
  `GITHUB_TOKEN`, so no personal token secret is required.
- **Dependabot** opens weekly Go dependency and GitHub Actions update PRs. It
  does not merge or release changes automatically. Action revisions are pinned
  to full commit SHAs. The GoReleaser CLI version is explicitly pinned in both
  workflows and should be updated in both places together.

## Cut a release

Before the first public release, enable GitHub private vulnerability reporting
and verify the report link in `SECURITY.md`. Check the repository description,
secret scanning/push protection settings, and public release downloads.
Require approval for workflows from external contributors in Actions settings.
Run the live smoke checks in `CONTRIBUTING.md` and record the Plane version tested.

`main` requires a pull request, all five CI checks, an up-to-date branch, and
resolved review conversations. These requirements apply to administrators too;
force pushes and branch deletion are blocked. Review approvals are optional so
maintainers can merge their own pull requests after the checks pass.

1. Merge the intended changes into `main` and wait for CI to pass.
2. Update `CHANGELOG.md` with the release's changes through a pull request.
3. Optionally test packaging locally using GoReleaser v2.18.1:

   ```sh
   goreleaser check
   goreleaser release --snapshot --clean
   ```

   Snapshot mode writes to `dist/` without publishing. Do not use snapshot
   binaries as official release assets.

4. Create and push an annotated tag at the release commit:

   ```sh
   git switch main
   git pull --ff-only
   git tag -a v0.2.0 -m 'Plane CLI v0.2.0'
   git push origin v0.2.0
   ```

5. Follow the **Release** workflow in GitHub Actions. Confirm that the release
   has six archives and `checksums.txt`, then download the archive for your
   platform, verify its checksum, and run `plane --version`.

Tags such as `v0.2.0-rc.1` become prereleases. Plain version tags become stable
releases. The release title is the tag itself, such as `v0.2.0`. Release notes
include the CLI summary and a generated commit changelog.

## Retry a failed release

Fix infrastructure or permissions, then rerun failed jobs on the same workflow
run. You may also dispatch the Release workflow against an existing tag:

```sh
gh workflow run release.yml --ref v0.2.0
```

Dispatching against a branch is rejected. GoReleaser builds all archives before
publishing and keeps existing release notes when retrying. If a published release
needs a code fix, publish a new patch version; do not move a published tag.

## Artifacts

| OS | Architectures | Archive |
| --- | --- | --- |
| Linux | amd64, arm64 | `.tar.gz` |
| macOS (`darwin`) | amd64, arm64 | `.tar.gz` |
| Windows | amd64, arm64 | `.zip` |

Archives contain `plane` (or `plane.exe`), the license, README, changelog, and
`third_party/` license notices generated for all six build targets. Packaging
fails if the license collector encounters an unreviewed unknown license.
The version is embedded in the executable during the build. Binaries are built
with CGO disabled. macOS binaries are not signed or notarized.

Repository visibility controls who can download the releases. For a private
repository, use an authenticated GitHub session or `gh release download`.

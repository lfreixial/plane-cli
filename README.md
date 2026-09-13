# plane-cli

A Go command-line client for [Plane](https://github.com/makeplane/plane). The executable is named `plane`.

Browse work items in an interactive terminal, create and update them from commands, and use JSON output in scripts. Supports Plane Cloud and self-hosted instances with the public v1 API.

## Install

Download prebuilt binaries from [GitHub Releases](https://github.com/lfreixial/plane-cli/releases/latest).
Linux and macOS archives use `.tar.gz`; Windows archives use `.zip`. Choose
`amd64` for Intel/AMD or `arm64` for ARM, including Apple Silicon. Each release
includes `checksums.txt` for verification. Go is not needed to run these binaries.

For example, on Linux amd64 with authenticated GitHub CLI access:

```sh
mkdir -p /tmp/plane-cli-install
gh release download v0.1.0 --repo lfreixial/plane-cli \
  --pattern 'plane-cli_0.1.0_linux_amd64.tar.gz' \
  --pattern checksums.txt --dir /tmp/plane-cli-install
cd /tmp/plane-cli-install
sha256sum --ignore-missing --check checksums.txt
tar -xzf plane-cli_0.1.0_linux_amd64.tar.gz
install -d "$HOME/.local/bin"
install -m755 plane "$HOME/.local/bin/plane"
plane --version
```

Ensure `~/.local/bin` is on your `PATH`. Private repository releases require
authenticated access. macOS binaries are not signed or notarized.

### Build from source

Requires Go 1.25 or newer. Run these commands from the repository folder.

```sh
make build
./bin/plane --help

# Optional: install to ~/.local/bin (add it to PATH)
make install
```

Without Make:

```sh
go build -o bin/plane ./cmd/plane
go install ./cmd/plane
```

`make build` and `make install` derive the version from Git: a clean `v0.1.0`
checkout reports `0.1.0`, later commits include their distance and commit hash
(for example, `0.1.0-1-g1133129`), and uncommitted changes add `-dirty`. Without
Git metadata, the version falls back to `dev`. Set `VERSION` explicitly when
building from a source archive, for example `make install VERSION=0.1.0`.
The plain `go build` and `go install` commands above use the default `dev` label.

`go install` writes to `GOBIN`, or `$(go env GOPATH)/bin` by default. The commands below assume `plane` is on your PATH; you can also run `./bin/plane`.

## Connect to Plane

Create a personal access token in Plane's profile settings, then run:

```sh
plane init
```

Setup asks for your API URL, workspace slug, API key (hidden input), and default project. It verifies workspace access before saving. A key entered at the hidden prompt is stored in the config file, with Unix permissions `0600`.

For environment-based credentials and unattended setup:

```sh
export PLANE_API_KEY='your-personal-access-token'
plane init --no-input --workspace my-team --project ENG
```

Environment tokens are **not saved** unless you pass `--save-token`. Keep `PLANE_API_KEY` available in subsequent shells, or supply it through your secret manager.

For a self-hosted instance:

```sh
plane init --url https://plane.example.com --workspace my-team
```

Use the instance origin, not its workspace page URL. An API base ending in `/api/v1` is also accepted. Plane Cloud defaults to `https://api.plane.so`; browser links use `https://app.plane.so`. If your web app and API have different origins, set `--web-url` during setup.

## Everyday workflow

```sh
plane project list
plane project use           # choose from an interactive project picker
plane project use ENG
plane project view          # view the currently selected project

plane issue list
plane issue list --state 'In Progress' --assignee alice@example.com
plane issue list --priority high --search login --plain

plane issue create --title 'Fix login redirect' --priority high \
  --state 'Todo' --assignee alice@example.com --label bug
plane issue create --title 'Investigate timeout' --body-file notes.txt

plane issue view ENG-42
plane issue edit ENG-42 --title 'Fix expired-session redirect' --target 2026-10-01
plane issue move ENG-42 'In Progress'
plane issue assign ENG-42 alice@example.com bob@example.com
plane issue comment add ENG-42 'Reproduced on staging.'
plane issue comment list ENG-42
plane issue open ENG-42

plane issue assign ENG-42 none
plane issue edit ENG-42 --label none --target none
plane issue delete ENG-42 --yes
```

- Work items accept keys such as `ENG-42` or UUIDs. UUIDs require a selected project; keys resolve their own project.
- Projects accept identifiers, exact names, or UUIDs. States, labels, cycles, and modules accept exact names or UUIDs. Matching is case-insensitive; ambiguous names require a UUID.
- `project view` uses the selected project when no reference is supplied. `project use` without a reference opens a searchable picker; use an explicit reference in scripts or with `--plain`/`--json`.
- Members accept display names, emails, or user UUIDs. `member list` shows the available IDs.
- `--assignee` and `--label` accept comma-separated or repeated values. Editing them replaces the whole list; `none` clears it.
- `--body` and `--body-file` accept plain text, safely converted to HTML. `--body-file -` reads stdin. An empty `--body=''` clears a description.
- Dates use `YYYY-MM-DD`. Work item dates can be cleared with `none`.
- Delete commands prompt for confirmation in a terminal and require `--yes` in scripts.

`work-item` and `work-items` are aliases for `issue`.

### Interactive browser

`plane issue list` opens the terminal browser when stdin and stdout are terminals. Use arrow keys or `j`/`k` to navigate, `/` to filter the loaded items, Enter for details, Escape to return, and `q` to quit. Detail pages scroll with the arrow keys. Resize the terminal freely.

Use `--plain` for a table or `--json` for JSON. Redirected output automatically uses a table. The browser is for reading; update work items using the commands above.

### Projects and workflow configuration

```sh
plane project create --name Engineering --identifier ENG
plane project edit ENG --description 'Product engineering'
plane project view ENG

plane state list
plane state create --name Review --group started --color '#8B5CF6'
plane state edit Review --color '#06B6D4'

plane label list
plane label create --name bug --color '#EF4444'
plane member list
plane member list --all
```

`project`, `state`, `label`, `cycle`, and `module` each support `list`, `view`, `create`, `edit`, and `delete`. Use `--help` for each command's fields.

### Cycles and modules

```sh
plane cycle create --name 'Sprint 12' --start 2026-10-01 --end 2026-10-14
plane cycle list
plane cycle add 'Sprint 12' ENG-42 ENG-43
plane cycle issues 'Sprint 12'
plane cycle remove 'Sprint 12' ENG-43

plane module create --name Authentication --status planned --target 2026-10-31
plane module add Authentication ENG-42
plane module issues Authentication
plane module edit Authentication --status in-progress
```

Membership commands require a selected project. All supplied work items are resolved and checked against that project before the write request is sent.

## Scripting and configuration

```sh
plane issue list --json --limit 0 | jq '.[] | {key, name, priority}'
plane issue create --title 'Automated task' --json | jq -r .id
plane issue open ENG-42 --print
plane -p OPS issue list --plain

plane config show  # effective values; token redacted
plane config path
```

List commands default to at most 100 results; `--limit 0` fetches all pages. Issue filters (`--state`, `--assignee`, `--priority`, `--search`) are applied locally across all pages, then the limit is applied. This supports servers whose public API does not expose those filters, but can be slower for large projects. Interactive `/` filtering only searches already loaded items. `--order-by` controls API ordering.

JSON lists are arrays, with a derived `key` field added to work item listings where the project identifier is available. Other object fields retain the API's response shape. Successful empty responses, including deletions, print `null` in JSON mode. Errors go to stderr and exit with status 1; successful commands exit with status 0.

Configuration precedence: **command flags > environment > config file > defaults**.

| Environment variable | Purpose |
| --- | --- |
| `PLANE_API_KEY` | Personal access token |
| `PLANE_URL` | API origin or base path |
| `PLANE_WEB_URL` | Web app origin for browser links |
| `PLANE_WORKSPACE` | Workspace slug |
| `PLANE_PROJECT` | Default project reference |
| `PLANE_CONFIG` | Alternate config file path |

The default file is `plane-cli/config.json` under Go's OS-specific user config directory (`$XDG_CONFIG_HOME`, or `~/.config` on Linux). `--config` chooses another file. `project use` changes the stored default; an active `PLANE_PROJECT` environment variable still overrides it.

HTTP requests default to a 30-second timeout, configurable with `--timeout 10s`. Short rate limits are retried twice for GET requests. Mutations are never automatically retried. Redirects are rejected to keep credentials on the configured origin; configure the final API URL if a reverse proxy redirects requests.

### Shell completion

```sh
source <(plane completion bash)
source <(plane completion zsh)
plane completion fish | source
```

PowerShell completion is also available through `plane completion powershell`.

## Development

```sh
make test       # tests with race detection; local HTTP mock servers, no Plane credentials
make vet
make build VERSION=0.1.0
```

The code is split into `internal/api` (HTTP and pagination), `internal/config` (configuration storage), and `internal/cli` (commands, resolution, output, and Bubble Tea browser). Command parsing and shell completion use Cobra. CI checks formatting, vet, tests, and the build.

See [RELEASING.md](RELEASING.md) for the tag-triggered release workflow,
local packaging checks, and release retry instructions. Dependabot opens weekly
dependency and Actions updates.

Tests cover API request formats, cursor pagination, rate limits, timeouts, redirect handling, reference resolution, field clearing, confirmation, configuration precedence, and browser navigation. A live Plane instance is not required by the tests; live compatibility still needs verification against your deployment.

## Scope and API references

This is an independent client, with an initial focus on daily project and work item management. Attachments, pages, custom fields, OAuth, and administrative operations are outside the current command set.

The implementation follows the [Plane API introduction](https://developers.plane.so/api-reference/introduction), [work item endpoints](https://developers.plane.so/api-reference/issue/overview), [identifier lookup](https://developers.plane.so/api-reference/issue/get-issue-sequence-id), and [cycle membership API](https://developers.plane.so/api-reference/cycle/add-cycle-work-items), checked against [Plane's API source](https://github.com/makeplane/plane/tree/preview/apps/api/plane/api).

Work item API calls use `/work-items/`, replacing the deprecated `/issues/` API paths. Cycle and module memberships retain their documented `/cycle-issues/` and `/module-issues/` paths and `issues` payload field. Browser URLs follow Plane's separate web routes.

MIT licensed. No Plane server source is bundled.

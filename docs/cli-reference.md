<!-- contentType: Reference · plan: docs/content-plan.md -->

# CLI reference

This page lists the commands, flags, environment variables, exit codes and JSON format of `plumb`. It matches the code in `apps/cli/cmd/plumb` and the output of `plumb <command> -h`.

## Commands

`plumb` has 9 commands. If you don't give a command, plumb prints help.

| Command | What it does |
| --- | --- |
| `plumb trace <vm>` | Traces a VM by UUID, name, fixed IP or floating IP |
| `plumb <vm>` | Shortcut for `plumb trace <vm>` |
| `plumb path <from> <to>` | Checks whether one VM can send traffic to another |
| `plumb link` | Remembers the OpenSDN URLs for the cloud in `OS_AUTH_URL`. Shows the current link when you give no flags |
| `plumb whoami` | Shows the user, project, roles and Config API URL, with where the URL came from |
| `plumb demo [scenario]` | Traces `web-01` in the lab built into the binary, without using the network |
| `plumb doctor` | Checks the credentials and every endpoint a trace needs |
| `plumb explain [code]` | Explains a warning code. If you don't give a code, it shows every code |
| `plumb version` | Prints the version |

If you mistype a command name, plumb suggests the closest one. `<vm>` can be a UUID, a name or an IP. For an IP, the `nova` stage searches Neutron for a port with that fixed IP. If it finds none, it searches floating IPs. It then uses the port's `device_id` as the VM. A `<vm>` that is neither a UUID nor an IP is a name. The `nova` stage searches for a server with that exact name in the token's project. If it finds none and the token has the `admin` role, it searches every project. You can put flags before or after `<vm>`.

## Demo scenarios

`plumb demo --list` shows this list:

| Scenario | Broken layer | Exit code |
| --- | --- | --- |
| `healthy` | None. This is the default | `0` |
| `devstack` | No Config API, so the 3 OpenSDN stages are skipped | `0` |
| `vmi-missing` | The Neutron port has no VMI in OpenSDN | `1` |
| `missing-route` | `control-02` has no route for the VM | `0` |
| `label-mismatch` | The agent uses a label the control node didn't advertise | `0` |
| `agent-down` | The agent introspect refuses the connection | `1` |

## OpenStack environment variables

`plumb <vm>` and `plumb doctor` read the same variables that openrc exports:

| Variable | Required | Default and notes |
| --- | --- | --- |
| `OS_AUTH_URL` | Yes | Adds `/v3` to the end of the URL if it's missing |
| `OS_USERNAME`, `OS_PASSWORD` | Yes, unless you use an application credential |  |
| `OS_PROJECT_NAME` or `OS_PROJECT_ID` | Yes, unless you use an application credential | Also reads `OS_TENANT_NAME` and `OS_TENANT_ID` instead |
| `OS_USER_DOMAIN_NAME` or `OS_USER_DOMAIN_ID` | No | `Default` |
| `OS_PROJECT_DOMAIN_NAME` or `OS_PROJECT_DOMAIN_ID` | No | `Default` |
| `OS_APPLICATION_CREDENTIAL_ID`, `OS_APPLICATION_CREDENTIAL_SECRET` | No | Used instead of a username and password |
| `OS_REGION_NAME` | No | Empty, which means plumb takes the endpoint of the first region in the catalog |
| `OS_INTERFACE` | No | `public`. Also accepts `publicURL` |

## `plumb path` checks

`plumb path` traces both VMs, then runs checks in the order a packet passes through.

| Check | Passes when | Needs |
| --- | --- | --- |
| `resolve` | Both VMs have a port in Neutron with an IPv4 address | Any cloud |
| `ports` | Both ports are `ACTIVE` | Any cloud |
| `network` | The destination IP is in the source's subnet, or one router connects both networks | Any cloud |
| `egress` | A security group of the source has an egress rule that allows traffic to the destination IP, or port security is off | Any cloud |
| `ingress` | A security group of the destination has an ingress rule that allows traffic from the source's IP or group, or port security is off | Any cloud |
| `route` | The source's VRF on the source compute node has a route to the destination IP | OpenSDN |
| `next-hop` | The route sends traffic into the destination's tap on the same compute node, or tunnels it to the destination's compute node with the same label as the destination's interface | OpenSDN |

Security groups are stateful, so plumb checks only egress on the source and ingress on the destination. ICMP means an echo request, like the one `ping` sends. `plumb path` doesn't yet check IPv6, floating IPs reached from outside the cloud or OpenSDN network policies directly.

| Flag | Default | What it does |
| --- | --- | --- |
| `--proto` | `icmp`, or `tcp` if you only set `--port` | The protocol to check: `icmp`, `tcp` or `udp` |
| `--port` | None | The destination port. Required with `tcp` or `udp` |

`plumb path` accepts the same endpoint flags, output flags, `--record` and `--replay` as `plumb trace`. It exits with code `1` when any check fails.

## `plumb link` flags

| Flag | What it does |
| --- | --- |
| `--config-url` | The Config API URL to remember |
| `--control-url` | The control introspect URLs to remember, separated by commas |
| `--remove` | Removes the link for this cloud |
| `--force` | Saves even if the Config API doesn't respond from this machine |

Before it saves, `plumb link` sends a `GET` to `--config-url`. If no HTTP response comes back, the command exits with code `1` and saves nothing.

## Where the OpenSDN URLs come from

`trace`, `doctor` and `whoami` take the Config API and control node URLs from the first place in this list that has a value:

1. The `--config-url` and `--control-url` flags
2. The `OPENSDN_CONFIG_URL` and `OPENSDN_CONTROL_URLS` variables
3. The link for the cloud that matches `OS_AUTH_URL` in the config file

The config file is at `$XDG_CONFIG_HOME/plumb/config.json`, or `~/.config/plumb/config.json` if `XDG_CONFIG_HOME` isn't set. The `PLUMB_CONFIG` variable changes the file's path. plumb treats `OS_AUTH_URL` values that differ only by a trailing `/v3` or `/` as the same cloud. It writes the file with permission `0600` because the file holds internal addresses.

## New version notices

After a command finishes, plumb tells you on stderr when a release newer than your version exists. plumb reads the latest release from a URL set at build time and caches the result in `$XDG_CACHE_HOME/plumb/update.json` or `~/.cache/plumb/update.json` for 24 hours. The check runs alongside the command and waits no more than 500 ms after the command finishes.

plumb doesn't check in these cases:

- stderr isn't a terminal
- `CI` or `PLUMB_NO_UPDATE_CHECK` is set
- You use `--json`
- The binary is version `dev`
- The binary was built without `UPDATE_URL`

The URL must respond with JSON in the GitHub releases API format, with `tag_name` and `html_url`, or in a format with `version` and `url`. The `PLUMB_UPDATE_URL` variable replaces the URL set at build time.

## Terminal environment variables

| Variable | Effect |
| --- | --- |
| `NO_COLOR` | Turns off color |
| `FORCE_COLOR` | Turns on color even when stdout isn't a terminal, unless the value is `0` |
| `TERM=dumb` | Turns off color |
| `CI` | Turns off the spinner |

plumb shows color when stdout is a terminal, and shows a spinner on stderr when stderr is a terminal.

## `plumb <vm>` and `plumb doctor` flags

Each OpenSDN flag reads its default from the environment variable in the third column:

| Flag | Default | Variable | What it does |
| --- | --- | --- | --- |
| `--config-url` | Empty | `OPENSDN_CONFIG_URL` | The Config API URL. If it's empty, plumb skips all 3 OpenSDN stages |
| `--control-url` | Found from `bgp-router` | `OPENSDN_CONTROL_URLS` | The control introspect URLs, separated by commas |
| `--agent-url` | The `virtual-router` IP and `--agent-port` | `OPENSDN_AGENT_URL` | The agent introspect URL. Used only by `plumb <vm>` |
| `--control-port` | `8083` |  | The port plumb uses when it finds control nodes itself |
| `--agent-port` | `8085` |  | The port plumb uses when it finds the agent itself |
| `--no-config-token` | Off |  | Doesn't send the Keystone token to the Config API |
| `--timeout` | `2m0s` |  | The maximum time for the whole command |
| `--request-timeout` | `15s` |  | The maximum time per HTTP call |
| `--insecure` | Off |  | Doesn't verify TLS certificates |

`plumb doctor` limits each probe to 3 seconds and probes up to 16 compute nodes at a time.

## Output flags

`plumb <vm>`, `plumb demo` and `plumb doctor` accept these flags:

| Flag | What it does |
| --- | --- |
| `--json` | Prints the result as JSON instead of a tree |
| `--no-color` | Turns off color |
| `--no-timings` | Doesn't show the time of each stage |
| `--debug` | Prints every HTTP call to stderr and turns off the spinner. `--dump-http` does the same |

## Recording flags

Only `plumb <vm>` accepts these 2 flags, and you can't use them together:

| Flag | What it does |
| --- | --- |
| `--record DIR` | Saves every response to `DIR` |
| `--replay DIR` | Answers every call from the files in `DIR`, without using the network |

If you use `--replay` without setting `OS_USERNAME`, plumb uses the value `replay` instead, because recordings don't depend on the request body.

## Recording files

`--record` creates 1 file per call. The file name comes from the method, host, path and query, following `httpx.Key`. The content is the HTTP response as text:

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"server": {…}}
```

Files keep only the `Content-Type` and `X-Subject-Token` headers, and `--record` writes `recorded-token` in place of the real `X-Subject-Token` value. Files don't store requests, so they contain no passwords. The `plumb demo` lab uses the same format and lives in `apps/cli/internal/demo/lab`.

## Exit codes

| Exit code | Meaning |
| --- | --- |
| `0` | No stage or check failed. There may be warnings |
| `1` | At least 1 stage or check failed |
| `2` | The command, an argument or a flag is invalid, or credentials are missing |

## Stage status

The `Steps` section shows each stage's status with one of 4 marks:

| Mark | JSON status | Meaning |
| --- | --- | --- |
| `✓` | `ok` | The API responded in full, and the data matches across layers |
| `!` | `warn` | The API responded, but the data doesn't match across layers |
| `✗` | `fail` | The stage's main call failed |
| `-` | `skip` | Data that an earlier stage should have found is missing |

## Trace JSON

`--json` prints the `Trace` struct in `apps/cli/internal/trace/model.go`. Each stage in `steps` has these fields:

| Field | Meaning |
| --- | --- |
| `name`, `status`, `duration_ms` | The stage's name, status and time |
| `error` | The error message when `status` is `fail`, or the reason when it's `skip` |
| `code`, `hint` | The code and what to check next, when `status` is `fail` and plumb knows the cause |
| `warnings[]` | Objects with `code`, `message` and `hint` |

`plumb explain` knows every `code` that appears in the JSON.

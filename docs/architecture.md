<!-- contentType: Conceptual · plan: docs/content-plan.md -->

# How plumb is built

This page explains plumb's package layout, the 7 decisions that shape the code and the reason for each. The steps to add a stage are in [Add a stage](add-a-stage.md).

## Package layout

The Go code is in `apps/cli/`, which is the module `github.com/PunGrumpy/plumb/apps/cli`. The code falls into 4 groups: a client for each API, the stages that link the data together, the output, and the commands that help users get started:

```text
apps/cli/
  cmd/plumb/              commands, flags, transport and exit codes
  internal/httpx/         HTTP client, --debug, --record, --replay
  internal/keystone/      token and service catalog
  internal/nova/          server and os-interface
  internal/glance/        image names
  internal/neutron/       port, network, subnet, security group, FIP
  internal/opensdn/
    config/               Config API (8082)
    sandesh/              generic introspect parser
    control/              control node introspect (8083)
    agent/                vRouter agent introspect (8085)
  internal/trace/         Trace struct, 6 stages and issue codes
  internal/render/        tree, verdict and JSON
  internal/ui/            colors and spinner
  internal/demo/          lab built into the binary, and scenarios
  internal/doctor/        checks access to every layer
```

Each client knows only its own API, and clients don't import each other. Only `apps/cli/internal/trace` knows which value from which layer goes to which layer.

## 7 decisions and their trade-offs

All 7 decisions favor showing each layer's data as clearly as possible, even when the code gets longer. This project exists to learn the system, so clarity matters more than line count.

### Write requests with `net/http` instead of gophercloud

plumb builds every request with the standard library. The project's goal is to show the URL, headers and microversion at each layer. gophercloud hides all 3 inside the library.

The trade-off is that the code must do work the library used to do, such as adding `/v3` to the end of `OS_AUTH_URL` and `/v2.0` to the end of the Neutron endpoint. There are only a few such places, so it's worth it to see the real requests.

### Every call goes through one `httpx.Client`

Every client makes HTTP calls through `httpx.Client`. Because of this, the following 3 features work for every layer without changing any client:

- `--debug` logs every call
- `--record` is an `http.RoundTripper` that saves responses to files
- `--replay` is an `http.RoundTripper` that answers from files

The recorded files are plain-text HTTP responses. You can write or edit them by hand, and `http.ReadResponse` can read them back. As a result, the lab in `apps/cli/internal/demo/lab` works both in `plumb demo` and in tests, without a mock server.

### `Trace` is the only contract between stages and output

Stages write results to the `Trace` struct in `apps/cli/internal/trace/model.go`, and renderers read only from that struct. So the tree, `--json` and a future web page all show the same data.

`Trace` stores results per port, because the port UUID is the only value that works from Nova all the way to the vRouter agent. A VM with 2 ports has 2 `Port` entries, and each one holds the results for every layer.

### A stage with a problem doesn't stop the next stage

Each stage ends with the status `ok`, `warn`, `fail` or `skip`, and then the next stage runs. What each status means is in [CLI reference](cli-reference.md).

This choice means the output always shows which layer the data reached and where it stopped, which is the question people open plumb to answer. If plumb stopped at the first error, you would see only the broken layer and not that the other layers are fine.

The `neutron` stage looks up ports by `device_id` and doesn't use Nova's result. So if the `nova` stage fails, the next stages still have ports to work with.

### Parse Sandesh into a tree instead of structs

Introspect responds with Sandesh XML, and field names can change between releases. So the `sandesh` package parses the XML into a tree of `Node`, and clients read fields by name with `Str`, `Int` and `Strings`.

If your release doesn't have a field, the value comes back empty, not as an error. You fix field names in `apps/cli/internal/opensdn/control` or `apps/cli/internal/opensdn/agent` without touching the parser. The downside is that the compiler doesn't warn you when a field name is wrong, so the built-in lab's tests must cover every field the tree shows.

### Read only, and never send the token to introspect

plumb sends one `POST` to Keystone to get a token. Every other call is a `GET`. plumb sends the token only to Nova, Glance, Neutron and the Config API. Introspect has no auth and accepts requests over unencrypted HTTP, so sending the token to introspect adds risk with no benefit.

### Every problem has a code, a hint and a command that explains it

Warnings and errors whose cause plumb knows have a fixed code, such as `route-missing`. Each code's meaning and what to check next are in `apps/cli/internal/trace/issues.go`. That data is used in 4 places: the `Hint` line at the end of the tree, the `plumb explain` command, the `code` and `hint` fields in JSON, and the sections in `docs/troubleshooting.md`.

I chose this because people who open plumb when something is broken need to know 2 things: what broke and what to do next. A Go error message covers only the first. The cost is that every new code needs a description and a section in the docs, so the test `TestEveryCodeIsDocumented` fails if a section is missing.

`plumb demo` follows the same idea. First-time users can see correct output before they have credentials, and each scenario shows what 1 kind of problem looks like. If you've seen `plumb demo missing-route`, you'll recognize the same output when you hit it on a real cloud.

## Side projects that can reuse this code

3 side projects can use plumb's packages directly:

- An OpenSDN object graph web page reads the output of `plumb --json` or calls `apps/cli/internal/opensdn/config` itself. Every `Ref` has the target's `to` and `uuid`, so it can build edges without another `GET`.
- A drift checker between Neutron and OpenSDN adds methods that list objects to the `neutron` and `config` clients, then compares port UUIDs with VMI UUIDs.
- A simulated control plane uses the structs in `apps/cli/internal/trace/model.go` as a model for messages between the controller and a simulated agent.

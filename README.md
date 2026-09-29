<!-- contentType: Landing · plan: docs/content-plan.md -->

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="brand/wordmark-white.svg">
    <img src="brand/wordmark-black.svg" alt="plumb" width="240">
  </picture>
</p>

# Follow a VM through every layer of OpenStack and OpenSDN

plumb is a Go CLI that takes a VM's name, UUID or IP and follows it from Keystone, Nova and Neutron down to the OpenSDN Config API, the control nodes and the vRouter agent on the VM's compute node. You get one tree that shows every layer.

The objects that link a VM to its route on the vRouter agent form a chain. When two layers disagree, plumb tells you where the chain stops, what to check next and which command explains the problem.

## What plumb shows

plumb calls read-only APIs and reports on the six layers below. Layers 5 and 6 are introspect: the HTTP debug pages that every OpenSDN process serves.

| Layer | Port | What plumb reads |
| --- | --- | --- |
| Keystone | from `OS_AUTH_URL` | Token, roles and endpoints from the service catalog |
| Nova | from the catalog | Host, flavor, image and attached ports |
| Neutron | from the catalog | Ports, networks, subnets, routers, security groups and floating IPs |
| OpenSDN Config API | `8082` | Virtual machine interface (VMI), virtual network (VN), routing instance (RI) and route target (RT) |
| Control node introspect | `8083` | The VM's routes and the Extensible Messaging and Presence Protocol (XMPP) session with the compute node |
| vRouter agent introspect | `8085` | Tap interface, virtual routing and forwarding (VRF) table, routes and flows |

This is the port section of a trace against the built-in lab, with long lines cut at `…`. Run `plumb demo` to see the full output:

```text
└─ Port      9c1e4d2b-7a3f-…  fa:16:3e:5a:12:7c  10.0.1.5
   ├─ Neutron   ACTIVE  vif_type=vrouter  host=compute-02
   ├─ Config    VMI default-domain:admin:9c1e4d2b-…  ✓ same UUID as the port
   │  ├─ RI        default-domain:admin:vn1:vn1
   │  └─ RT        target:64512:8000002  import+export
   ├─ Control   control-01  vn1:vn1.inet.0  10.0.1.5/32 ✓
   │  └─ path      XMPP from compute-02  nh 10.10.0.21  label 25  encap gre,udp
   └─ vRouter   tap9c1e4d2b-7a ✓ active  vrf vn1:vn1 (index 3)  label 25
      └─ route     10.0.1.5/32  local interface tap9c1e4d2b-7a  label 25
```

## Get started

Build plumb with Go 1.24 or later, then trace a VM in the lab that ships inside the binary. You don't need a cloud or credentials for this step:

```sh
make -C apps/cli build
./apps/cli/bin/plumb demo
```

On a real cloud, source your openrc and use the commands in this order:

| Command | Use it when |
| --- | --- |
| `plumb demo [scenario]` | You want to see a working trace, or how a broken layer looks |
| `plumb link --config-url <url>` | You use a cloud that runs OpenSDN for the first time, so plumb remembers its Config API |
| `plumb doctor` | You run plumb on a machine for the first time, to see which layers it reaches |
| `plumb <vm>` | You trace a VM by name, UUID, fixed IP or floating IP |
| `plumb path <from> <to>` | Someone reports that two VMs can't reach each other |
| `plumb explain <code>` | A trace ends with a `More` line and you want the details |
| `plumb whoami` | You want to see the user, project and OpenSDN URLs plumb uses |

To install plumb on a server, run `make -C apps/cli dist` and copy the static binary for its platform from `apps/cli/dist/`.

## Documentation

The guides are in `docs/`:

| Guide | Read it when |
| --- | --- |
| [Run plumb for the first time](docs/quickstart.md) | You want a step-by-step tour of the output before you use a real cloud |
| [Use plumb with DevStack and OpenSDN](docs/run-against-a-lab.md) | You install plumb on a server, link a cloud or record a lab |
| [CLI reference](docs/cli-reference.md) | You look up a command, flag, environment variable, exit code or JSON field |
| [How a VM's request crosses each layer](docs/concepts.md) | You want to understand the Neutron plugin, the schema transformer, XMPP, BGP, VRFs and the overlay |
| [What each warning means](docs/troubleshooting.md) | You get a code you don't know |
| [APIs plumb calls](docs/api-reference.md) | You compare endpoints and fields with your lab |
| [How plumb is built](docs/architecture.md) | You change the code |
| [Add a stage](docs/add-a-stage.md) | You add a new API to the trace |
| [Release plumb](docs/releasing.md) | You cut a release |

## Before you use plumb on a real environment

OpenSDN introspect has no authentication. Ask the environment owner for access to ports `8082`, `8083` and `8085` from the machine that runs plumb. plumb never sends your Keystone token to introspect, and it never calls an API that changes state.

## Brand

The logo, icons and usage rules are in [brand/README.md](brand/README.md).

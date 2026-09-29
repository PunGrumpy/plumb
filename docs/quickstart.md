<!-- contentType: Tutorial · plan: docs/content-plan.md -->

# Run plumb for the first time

In this tutorial you build plumb, then use `plumb demo` to follow the VM `web-01` from Keystone to the vRouter agent in the lab that ships inside the binary. Then you open a scenario where one route is missing, and you see how plumb points to it and tells you what to check next. Every step runs on your machine. You don't need a cloud or credentials.

## Build plumb

You need Go 1.24 or later. From the root of the repo, build the binary:

```sh
make -C apps/cli build
```

Check that the binary works:

```sh
./apps/cli/bin/plumb version
```

The output is `plumb` followed by the version, for example `plumb dev`.

## Run the built-in lab

Now run the demo:

```sh
./apps/cli/bin/plumb demo
```

The first line names the scenario that runs, followed by a tree that starts with `VM web-01`. The end of the output looks like this:

```text
Steps
  ✓ keystone            0 ms
  ✓ nova                0 ms
  ✓ neutron             0 ms
  ✓ opensdn-config      0 ms
  ✓ control             0 ms
  ✓ vrouter             0 ms

✓ Traced web-01 through 6 of 6 stages in 0 ms
```

You gave plumb the name `web-01`, not a UUID, so the `nova` stage looks up the UUID first. If your terminal shows color, the `✓` marks are green.

## Find the VM's port in the tree

Now scroll up to the `Port` line, which is the last part of the tree. This example cuts long lines at `…`:

```text
└─ Port      9c1e4d2b-7a3f-…  fa:16:3e:5a:12:7c  10.0.1.5
   ├─ Neutron   ACTIVE  vif_type=vrouter  host=compute-02
   ├─ Config    VMI default-domain:admin:9c1e4d2b-…  ✓ same UUID as the port
   ├─ Control   control-01  vn1:vn1.inet.0  10.0.1.5/32 ✓
   ├─ Control   control-02  vn1:vn1.inet.0  10.0.1.5/32 ✓
   └─ vRouter   tap9c1e4d2b-7a ✓ active  vrf vn1:vn1 (index 3)  label 25
```

Under the port there is one line per layer, from Neutron down to the vRouter agent. Find the `✓` mark in these 3 lines:

1. The `Config` line: `✓ same UUID as the port`
2. The `Control` lines for `control-01` and `control-02`: `10.0.1.5/32 ✓`
3. The `vRouter` line: `✓ active`

[How a VM's request crosses each layer](concepts.md) explains which value links each line to the next.

## Run it as DevStack

DevStack uses OVN, so it has no OpenSDN Config API to call. The `devstack` scenario runs the same lab without the Config API:

```sh
./apps/cli/bin/plumb demo devstack
```

The end of the output changes to this:

```text
  - opensdn-config   skipped: ports use vif_type ovs, so this cloud does …
  - control          skipped: needs opensdn-config
  - vrouter          skipped: needs opensdn-config

✓ Traced web-01 through 3 of 6 stages in 0 ms
  Skipped opensdn-config: ports use vif_type ovs, so this cloud does not …
```

The port in this scenario has `vif_type` set to `ovs`, so plumb says this cloud doesn't use OpenSDN and skips all 3 OpenSDN stages. The first 3 stages still run in full.

## Remove a route from one control node

Next you open a scenario where `control-02` has no route for the VM:

```sh
./apps/cli/bin/plumb demo missing-route
```

The `Control` line for `control-02` in the tree changes to `10.0.1.5/32 ✗ missing`, and the output ends with these 3 lines:

```text
! Traced web-01 through 6 of 6 stages in 0 ms, 1 issue
  Hint  Check that the port's vRouter line shows active, then check …
  More  plumb explain route-missing
```

The `Hint` line tells you what to check next, and the `More` line gives the command that explains this problem. Run that command:

```sh
./apps/cli/bin/plumb explain route-missing
```

plumb prints what `route-missing` means, what to check and the section in [What each warning means](troubleshooting.md).

## Look at the other scenarios

As a last step, list all scenarios:

```sh
./apps/cli/bin/plumb demo --list
```

Each scenario breaks one layer. Run `agent-down` and see that the mark in front of the last line changes to `✗` and the command ends with exit code `1`, because the `vrouter` stage can't call its API. `missing-route` ends with exit code `0`, because every API answers normally and only the data doesn't match.

## Next steps

You can now run plumb, read the tree and follow a hint. The next page depends on what you want to do:

- [Use plumb with DevStack and OpenSDN](run-against-a-lab.md)
- [What each warning means](troubleshooting.md)

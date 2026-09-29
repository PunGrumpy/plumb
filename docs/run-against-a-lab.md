<!-- contentType: How-to · plan: docs/content-plan.md -->

# Use plumb with DevStack and OpenSDN

This page shows how to install plumb on a server, check that the server reaches every layer, and run plumb against 2 kinds of real cloud: DevStack with OVN and a lab with OpenSDN. All flags and variables are in the [CLI reference](cli-reference.md).

## Install on a server

plumb needs to reach the vRouter agent introspect on every compute node, so run it from a bastion or controller on the management network.

1. Build the static binaries on your machine:

   ```sh
   make -C apps/cli dist
   ```

   This command creates `apps/cli/dist/plumb-linux-amd64`, `apps/cli/dist/plumb-linux-arm64` and `apps/cli/dist/plumb-darwin-arm64`. If you want the binary to tell you when a new version is out, pass a URL that returns the latest release:

   ```sh
   make -C apps/cli dist VERSION=v0.1.0 \
   	UPDATE_URL=https://api.github.com/repos/your_org/plumb/releases/latest
   ```

2. Copy the binary that matches the server:

   ```sh
   scp apps/cli/dist/plumb-linux-amd64 your_user@your_bastion:~/bin/plumb
   ```

The binary needs no other libraries, so you don't need to install Go on the server.

## Check that the server reaches every layer

1. On the server, source the openrc of the project that the VM is in:

   ```sh
   source ~/admin-openrc
   ```

2. If the cloud uses OpenSDN, link this cloud to its Config API:

   ```sh
   plumb link --config-url http://config_node_ip:8082
   ```

   plumb saves this URL with the cloud's `OS_AUTH_URL` in its config file, `~/.config/plumb/config.json` by default. [CLI reference](cli-reference.md) explains how `XDG_CONFIG_HOME` and `PLUMB_CONFIG` change the path. The `trace` and `doctor` commands then use this URL each time you source this cloud's openrc.

3. Run `doctor`:

   ```sh
   plumb doctor
   ```

`doctor` gets a token from Keystone, then calls every endpoint that a trace needs, including the vRouter agent on every compute node. The last line tells you which layer a trace from this server can reach. Compute nodes that plumb can't reach are listed under the `vrouter` line.

If `doctor` ends with `✗`, follow the `Hint` line before you run a trace.

## Run against DevStack

DevStack uses OVN, so you don't need to link a Config API. Pass the VM's name or UUID:

```sh
plumb your_vm_name
```

In the tree, the port's `Neutron` line must show `vif_type=ovs`. If it shows `vif_type=vrouter`, this cloud uses OpenSDN. Follow the next section.

If more than one VM has the same name, plumb shows the UUID of each one. Run it again with the UUID you want.

## Run against an OpenSDN lab

After you link the cloud as in "Check that the server reaches every layer", run:

```sh
plumb your_vm_name
```

plumb finds the control nodes from the `bgp-router` objects and the vRouter agents from the `virtual-router` objects in the Config API.

If you're not sure which URLs plumb uses, run `plumb whoami`. It shows the URLs and whether each one comes from `plumb link`, an environment variable or a flag.

If the `control` or `vrouter` stage fails because the server can't reach the IPs saved in the config, set the URLs yourself:

```sh
plumb your_vm_name \
	--control-url http://control_1_ip:8083,http://control_2_ip:8083 \
	--agent-url http://compute_mgmt_ip:8085
```

If you don't want to send your Keystone token to the Config API, add `--no-config-token`.

## Check that one VM can send traffic to another

If someone reports that VM A can't talk to VM B, pass both VMs and the protocol they use:

```sh
plumb path web-01 db-01 --port 5432
```

plumb checks the ports, routers and security groups on both sides, and on OpenSDN it checks the route and next hop in the source VRF. The last line names the first check that fails, and the `Hint` line gives a command that fixes it, for example `openstack security group rule create …`. If you pass `--port` without `--proto`, plumb checks TCP. If you pass neither, it checks ICMP like `ping`.

On DevStack with OVN, the `route` and `next-hop` checks are skipped, so the last line says that Neutron allows the traffic, not that the datapath delivers it.

## Record a lab to open offline

1. Run against the real lab and record the responses:

   ```sh
   plumb your_vm_name --record lab-recordings/web-01
   ```

2. Copy the directory back to your machine, then open the result with the same `OS_AUTH_URL` and flags you used to record:

   ```sh
   plumb your_vm_name --replay lab-recordings/web-01
   ```

The files in `lab-recordings/` contain the lab's internal IPs and project names. Don't commit these files. The repo's `.gitignore` already excludes this directory.

## Compare introspect field names with your lab

Introspect request and field names can change between OpenSDN releases. Do this the first time you use plumb with a new lab.

1. Record the lab as in the previous section.
2. Open the files whose names start with `GET_` followed by the IP of a control node or compute node.
3. Compare the element names in the files with the tables in [APIs plumb calls](api-reference.md).
4. If a name doesn't match, change it in `apps/cli/internal/opensdn/control/control.go` or `apps/cli/internal/opensdn/agent/agent.go`.

You can also open `http://control_node_ip:8083/` in a browser. That page lists every request the process serves.

## Send the output to other programs

To use the output in a script, use `--json` and read it with `jq`. This command shows the code of every warning:

```sh
plumb your_vm_name --json | jq -r '.steps[].warnings[]?.code'
```

Scripts can check the exit code. `1` means at least 1 stage failed.

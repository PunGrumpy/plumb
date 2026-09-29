<!-- contentType: Reference -->

# Content plan

This page is the plan for every doc page in this repo. It says what each page is for, who it's written for, and which questions are still open.

## Overview and readers

These docs are written for 2 groups of readers:

- OJT trainees who want to understand which OpenStack and OpenSDN layers a VM goes through. These docs support OJT item 11 on DevStack, item 13 on the OpenSDN and OpenStack API Tutorial, and item 19 on Python Clean OpenStack.
- Cloud operators who want to know which layer a VM's chain stops at, without opening introspect one page at a time.

## Reader goals

After reading everything, readers should be able to:

1. Run plumb against the built-in lab, DevStack and an OpenSDN lab.
2. Explain which UUIDs and object names link each layer together.
3. Explain why the Config API is separate from the control node.
4. Interpret each warning and pick the API to check next.
5. Add a stage or write a new UI that reads plumb's JSON.

## Each doc page

Each page does one job, based on its content type:

| Page | Content type | Goal |
| --- | --- | --- |
| [README](../README.md) | Landing | Pick the next page to read |
| [Run plumb for the first time](quickstart.md) | Tutorial | Run plumb and read the output |
| [Use plumb with DevStack and OpenSDN](run-against-a-lab.md) | How-to | Run against a real cloud and record a lab |
| [Add a stage](add-a-stage.md) | How-to | Add a new API to the trace |
| [Release plumb](releasing.md) | How-to | Release a new version and binaries |
| [CLI reference](cli-reference.md) | Reference | Look up flags, variables and exit codes |
| [How a VM's request crosses each layer](concepts.md) | Conceptual | Explain each layer to someone else |
| [How plumb is built](architecture.md) | Conceptual | Explain the reasons behind the code layout |
| [APIs plumb calls](api-reference.md) | Reference | Check endpoints and fields |
| [What each warning means](troubleshooting.md) | Troubleshooting | Find the cause when the chain stops |

## Open questions

Confirm these against a real OpenSDN lab before you use plumb on the team's environment:

- Do the Sandesh parameter and field names on the team's release match those in [APIs plumb calls](api-reference.md)?
- Does the Config API on the lab have keystone auth turned on?
- Who allows the machine that runs plumb to reach ports 8082, 8083 and 8085? Ask Moo.
- The data in `apps/cli/internal/demo/lab` is hand-written to match the API formats. It isn't recorded from a real lab yet.

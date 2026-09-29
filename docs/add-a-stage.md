<!-- contentType: How-to · plan: docs/content-plan.md -->

# Add a stage

This page shows how to add a stage that reads a new API, such as Octavia or Cinder, and shows the result in the tree. The reasons for this structure are in [How plumb is built](architecture.md).

1. If the API has no client yet, create a new package under `apps/cli/internal/` that takes a `*httpx.Client` and makes calls through `httpx.Service`.
2. Add a field to `Trace` or `Port` in `apps/cli/internal/trace/model.go` for the stage's result.
3. Write a method on `run` with the signature `func(context.Context) error`:
   - If data from an earlier stage is missing, return the value from `skip`.
   - If the API responds but its data doesn't match another layer, call `r.warn` with a code, such as `r.warn("route-missing", …)`.
   - If the stage's main call fails, return an error, or return the value from `fail` if you know the cause and have a code for it.
4. If you use a new code, add the code, its meaning and what to check to `explanations` in `apps/cli/internal/trace/issues.go`. Then add a `` ### `code` `` heading to `docs/troubleshooting.md`.
5. Add the stage to the `stages` table in `Run` in `apps/cli/internal/trace/trace.go`, in the order its data requires.
6. Show the result in `apps/cli/internal/render/tree.go`.
7. Add response files for the new API to `apps/cli/internal/demo/lab`. Each file name must match the request's `httpx.Key`. If a name is wrong, the stage fails with the error `replay: no recording`, followed by the file name it needs.
8. Regenerate the golden file and check the diff:

   ```sh
   make -C apps/cli golden
   git diff apps/cli/cmd/plumb/testdata/demo.golden
   ```

If the diff shows only the new stage's lines, run `make -C apps/cli test` and commit all the files. If you want users to see the new stage's problems in `plumb demo`, add a scenario to `apps/cli/internal/demo/demo.go` and add a case to `TestDemoScenarios`.

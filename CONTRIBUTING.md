# Contributing

## Branches

`Version2` is the default branch and the release line. Registry releases continue to come from `Version2` until Justin renames it to `main` or fast-forwards `main` and switches the default.

`soft/dev` is the v3 integration tip. Feature and fix work for v3, including the move onto govergeos, targets `soft/dev`.

Squash-merge pull requests into `soft/dev`. Merging `soft/dev` into `main` or `Version2` happens only with Justin's explicit sign-off.

`main` is behind `Version2` and is not the integration branch. Whether to make `main` the release line, and whether to cut `release/2.x` from the last 2.x tag, is still open.

## Tests

Unit tests do not need a VergeOS system or any lab secret:

```
go test ./...
```

`.github/workflows/test.yml` runs that suite on every pull request and on pushes to `soft/dev` and `Version2`, along with `go build`, `go vet`, `golangci-lint`, and a docs check. The docs check installs Terraform 1.16.4 (the generator's own download fails on an expired HashiCorp signing key) and runs `tfplugindocs generate`. It fails if the working tree changes. Page text lives in `templates/`; the committed files under `docs/` are the render. The templates currently keep the hand-written examples and attribute notes, so generate does not replace those notes with the shorter schema descriptions. The same unit command also runs with `TF_ACC_TERRAFORM_PATH` pointed at OpenTofu. Unit tests do not launch a CLI; acceptance tests do.

Acceptance tests talk to a lab. They stay skipped unless `TF_ACC=1` and these variables are set:

```
TF_ACC_VERGEIO_HOST
TF_ACC_VERGEIO_USERNAME
TF_ACC_VERGEIO_PASSWORD
```

Do not commit credentials. Every object a test creates must use `acctest.Name` so the name starts with `tf-acc-`. `make sweep` (or `TF_ACC=1 go test ./internal/acctest -run TestSweep -count=1`) deletes leftovers with that prefix and then checks that none remain. The acceptance workflow runs the sweep before and after `TestAcc`, on `workflow_dispatch`. It skips when the lab secrets are absent. A nightly schedule is commented in that workflow until the secrets exist.

Network, VM, and user acceptance tests cover create, an empty plan, an update, `ImportState` with `ImportStateVerify`, and destroy. Cloud-init file, member, and tag member acceptance tests are still skipped in code. Data sources have read-only acceptance tests and are not part of the sweep. Drives, NICs, and devices are nested in the VM resource and do not have their own acceptance test yet.

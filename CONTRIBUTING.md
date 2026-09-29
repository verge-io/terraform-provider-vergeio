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

`.github/workflows/test.yml` runs that suite on every pull request and on pushes to `soft/dev` and `Version2`, along with `go build`, `go vet`, `golangci-lint`, and a docs check. The docs check installs Terraform 1.16.4 (the generator's own download fails on an expired HashiCorp signing key) and runs `make generate`. That target runs the `tfplugindocs` version pinned in `tools/`. The job fails if the working tree changes. Page text lives in `templates/`; the committed files under `docs/` are the render. The templates currently keep the hand-written examples and attribute notes, so generate does not replace those notes with the shorter schema descriptions. The same unit command also runs with `TF_ACC_TERRAFORM_PATH` pointed at OpenTofu. Unit tests do not launch a CLI; acceptance tests do.

Acceptance tests talk to a lab. They stay skipped unless `TF_ACC=1` and these variables are set:

```
TF_ACC_VERGEIO_HOST
TF_ACC_VERGEIO_USERNAME
TF_ACC_VERGEIO_PASSWORD
```

Do not commit credentials. Every object a test creates must use `acctest.Name` so the name starts with `tf-acc-`. `make sweep` (or `TF_ACC=1 go test ./internal/acctest -run TestSweep -count=1`) deletes leftovers with that prefix and then checks that none remain. The acceptance workflow runs the sweep before and after `TestAcc`, on `workflow_dispatch`. It skips when the lab secrets are absent. A nightly schedule is commented in that workflow until the secrets exist.

Network, VM, and user acceptance tests cover create, an empty plan, an update, `ImportState` with `ImportStateVerify`, and destroy. Cloud-init file, member, and tag member acceptance tests are still skipped in code. Data sources have read-only acceptance tests and are not part of the sweep. Drives, NICs, and devices are nested in the VM resource and do not have their own acceptance test yet.

## Local development

`go install .` builds `terraform-provider-vergeio` and puts it on the Go bin path (`$(go env GOBIN)` when that is set, otherwise `$(go env GOPATH)/bin`).

Terraform loads that binary through a CLI configuration file. The default file is `~/.terraformrc`. `TF_CLI_CONFIG_FILE` overrides the path. The override key is the provider address in `main.go`, `vergeio/cloud/vergeio`. The value is the directory that contains the binary, not the binary itself.

```
provider_installation {
  dev_overrides {
    "vergeio/cloud/vergeio" = "/home/you/go/bin"
  }

  direct {}
}
```

`dev_overrides` skips the registry and the provider checksum for that source, so `terraform init` does not download a published build. `direct {}` keeps every other provider on the normal registry install path. A configuration that requests a different source, such as the `verge-io/vergeio` address in the published docs, needs that source as the override key instead.

## Releases

Pushing a `v*` tag runs `.github/workflows/release.yml`. That workflow runs `goreleaser release --clean` with `.goreleaser.yml`, which cross-compiles the provider and publishes the GitHub release.

To produce the same archives locally without publishing or signing:

```
goreleaser release --snapshot --clean --skip=sign
```

GoReleaser writes them under `dist/`. Snapshot builds are for checking the release config. They are not a substitute for `go install` when you are iterating on a change.

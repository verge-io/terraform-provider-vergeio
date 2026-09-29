# Contributing

## Branches

`Version2` is the default branch and the release line. Registry releases continue to come from `Version2` until Justin renames it to `main` or fast-forwards `main` and switches the default.

`soft/dev` is the v3 integration tip. Feature and fix work for v3, including the move onto govergeos, targets `soft/dev`.

Squash-merge pull requests into `soft/dev`. Merging `soft/dev` into `main` or `Version2` happens only with Justin's explicit sign-off.

`main` is behind `Version2` and is not the integration branch. Whether to make `main` the release line, and whether to cut `release/2.x` from the last 2.x tag, is still open.

## Tests

Unit tests do not need a VergeOS system:

```
go test ./...
```

Acceptance tests talk to a lab and stay gated on `TF_ACC`. They also need `TF_ACC_VERGEIO_HOST`, `TF_ACC_VERGEIO_USERNAME`, and `TF_ACC_VERGEIO_PASSWORD`. Do not commit credentials.

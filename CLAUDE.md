# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Terraform provider for VergeOS, enabling infrastructure-as-code management of VergeOS resources (VMs, networks, users, etc.). Built using Terraform Plugin Framework v1.

## Build & Development Commands

```bash
# Build the provider binary
make build

# Build and install to ~/.terraform.d/plugins/vergeio/cloud/vergeio/{VERSION}/{OS_ARCH}/
make install

# Run linter (requires golangci-lint)
make lint

# Generate Terraform documentation
make generate

# Run unit tests
go test ./...

# Run acceptance tests (requires TF_ACC=1 and valid VergeOS credentials)
TF_ACC=1 go test ./internal/provider/... -v -timeout 120m

# Build for all platforms
make release
```

## Testing Locally

1. Run `make install` to build and install the provider locally
2. Create a test Terraform configuration using provider source `vergeio/cloud/vergeio`
3. Run `terraform init && terraform apply`

## Architecture

### Directory Structure

- `main.go` - Provider entry point, serves gRPC protocol v6
- `internal/provider/provider.go` - Provider configuration and registration
- `internal/provider/vergeio/` - Shared client library (HTTP client, field cache, errors)
- `internal/provider/{resource}/` - Each resource/data source in its own package

### Resource Implementation Pattern

Each resource follows a consistent pattern with separate files:
- `{name}_resource.go` - Schema definition, CRUD lifecycle methods
- `{name}_api.go` - API client calls, request/response handling
- Optional: plan modifiers for computed fields (e.g., `machine_type_plan_modifier.go`)

Data sources follow similar pattern:
- `{name}_data_source.go` - Schema and Read method
- `{name}_api.go` - API calls

### Core Client (`internal/provider/vergeio/`)

- `client.go` - HTTP client with basic auth, TLS handling, CRUD helpers
- `field_cache.go` - Session-scoped lazy-loading cache for dynamic field values (machine types, disk interfaces, etc.)
- API endpoint format: `https://{host}/api/v4/{endpoint}`

### Provider Configuration

Required provider attributes: `host`, `username`, `password`
Optional: `insecure` (for self-signed certificates)

### Current Resources

`vergeio_vm`, `vergeio_network`, `vergeio_user`, `vergeio_member`, `vergeio_tag_members`

### Current Data Sources

`vergeio_vms`, `vergeio_networks`, `vergeio_clusters`, `vergeio_groups`, `vergeio_mediasources`, `vergeio_nodes`, `vergeio_version`, `vergeio_cloudinit_files`, `vergeio_resource_groups`, `vergeio_tags`

## Adding New Resources

1. Create package under `internal/provider/{name}/`
2. Implement resource file with schema and CRUD methods
3. Implement API file for VergeOS API interactions
4. Register in `provider.go` Resources() or DataSources()
5. Add examples in `examples/resources/{name}/` or `examples/data-sources/{name}/`
6. Run `make generate` to update documentation

## Debugging

Run provider with debug flag for IDE debugging:
```bash
go run main.go -debug
```

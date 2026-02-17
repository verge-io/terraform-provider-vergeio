# Architecture Decision Records

This document captures key design decisions made during the development of goVergeOS.

---

## ADR-001: Flat Package Structure

**Date:** 2025-01-02

**Status:** Accepted

**Context:** Go SDKs can be organized with nested packages (e.g., `vergeos/vm`, `vergeos/network`) or as a flat single package.

**Decision:** Use a flat package structure with all code in the root `vergeos` package.

**Rationale:**
- Simpler import paths for users (`vergeos.NewClient()` vs `vergeos.New()` + `vm.Service`)
- Follows patterns of popular Go SDKs (aws-sdk-go, google-cloud-go client libraries)
- Reduces package coupling issues
- Easier to maintain type relationships across services

**Consequences:**
- All types share the same namespace (prefix types if conflicts arise)
- Single `go.mod` file simplifies dependency management

---

## ADR-002: FlexInt Type for ID Handling

**Date:** 2025-01-02

**Status:** Accepted

**Context:** The VergeOS API inconsistently returns IDs as either integers or strings depending on the endpoint and context.

**Decision:** Create a custom `FlexInt` type that implements `json.Unmarshaler` to handle both string and integer JSON values.

**Rationale:**
- Provides seamless handling of API inconsistencies
- Users always work with `int` values regardless of API response format
- Encapsulates parsing logic in one place

**Consequences:**
- Slight overhead for JSON unmarshaling
- Type must be used for all ID fields that may have inconsistent API responses

---

## ADR-003: Functional Options Pattern for Client Configuration

**Date:** 2025-01-02

**Status:** Accepted

**Context:** The SDK client requires multiple configuration options (base URL, credentials, TLS settings, timeouts).

**Decision:** Use the functional options pattern (`WithBaseURL()`, `WithCredentials()`, etc.).

**Rationale:**
- Clean, readable API for users
- Easy to add new options without breaking changes
- Optional parameters have sensible defaults
- Self-documenting code

**Consequences:**
- Slightly more verbose than a config struct for simple cases
- Each option requires its own function

---

## ADR-004: Pointer Types for Optional Update Fields

**Date:** 2025-01-02

**Status:** Accepted

**Context:** Update requests need to distinguish between "not provided" and "set to zero/empty value".

**Decision:** Use pointer types (`*string`, `*int`, `*bool`) for all fields in Update request structs.

**Rationale:**
- `nil` pointer = field not included in request (don't update)
- Non-nil pointer = update field to this value (including zero values)
- Matches common Go SDK patterns (AWS, GCP)

**Consequences:**
- Slightly more verbose for users (need to use `&value` or helper functions)
- Clear semantics for partial updates

---

## ADR-005: Service-Oriented Architecture

**Date:** 2025-01-02

**Status:** Accepted

**Context:** Need to organize API operations in a way that's intuitive and maintainable.

**Decision:** Group operations by resource type into service structs (VMService, NetworkService, etc.) accessed via the main Client.

**Rationale:**
- Intuitive API: `client.VMs.List()`, `client.Networks.Create()`
- Easy to find related operations
- Services can maintain their own state if needed
- Follows patterns of GitHub, Stripe, and other popular SDKs

**Consequences:**
- Client struct holds references to all services
- Adding new resources requires adding new service

---

## ADR-006: Explicit Field Selection

**Date:** 2025-01-02

**Status:** Accepted

**Context:** VergeOS API supports field selection to optimize response payloads.

**Decision:** Define default field lists (`vmListFields`, `vmGetFields`) and use them automatically, while allowing user override via `WithFields()` option.

**Rationale:**
- Optimizes API calls by default
- Users don't need to know field names for common operations
- Power users can customize when needed

**Consequences:**
- Must maintain field lists as API evolves
- Different field sets for List vs Get operations

---

## ADR-007: Drive Interface Default Changed to virtio-scsi

**Date:** 2025-01-02

**Status:** Accepted

**Context:** The VergeOS schema indicates `virtio-scsi` is now the recommended default disk interface, replacing legacy `virtio`.

**Decision:** Document `virtio-scsi` as the default/recommended interface in SDK type comments.

**Rationale:**
- Aligns with VergeOS schema defaults
- Better performance and feature support
- Legacy `virtio` still available for compatibility

**Consequences:**
- Documentation updated to reflect new default
- Existing code using `virtio` continues to work

---

## ADR-008: Action Methods Return Error Only

**Date:** 2025-01-02

**Status:** Accepted

**Context:** VM actions (Clone, Snapshot, Reset, etc.) are asynchronous operations that may not return immediate results.

**Decision:** Action methods return only `error`, not the resulting resource.

**Rationale:**
- Actions are fire-and-forget at the API level
- Clone/Snapshot create new resources with different IDs
- Users can query for results separately if needed
- Simpler, more honest API

**Consequences:**
- Users must query separately to get cloned VM or snapshot details
- Consistent behavior across all action methods

---

## ADR-009: Context Support for All Operations

**Date:** 2025-01-02

**Status:** Accepted

**Context:** Go best practices recommend using `context.Context` for cancellation and timeouts.

**Decision:** All SDK methods that make API calls accept `context.Context` as their first parameter.

**Rationale:**
- Enables request cancellation
- Supports timeouts
- Follows Go idioms and standard library patterns
- Required for production-grade applications

**Consequences:**
- All method signatures include context parameter
- Users must pass context (can use `context.Background()` for simple cases)

---

## ADR-010: MIT License

**Date:** 2025-01-02

**Status:** Accepted

**Context:** Need to choose an open source license for the SDK.

**Decision:** Use MIT License.

**Rationale:**
- Permissive license encourages adoption
- Compatible with most other licenses
- Simple and well-understood
- Standard for SDK/library projects

**Consequences:**
- Users can use SDK in proprietary projects
- No copyleft obligations

---

## ADR-011: "goVergeOS" Naming Convention

**Date:** 2026-01-21 (Updated: 2026-01-29)

**Status:** Accepted

**Context:** Verge is standardizing library naming across all language-specific packages:
- **PSVergeOS** - PowerShell module
- **pyvergeos** - Python package (lowercase module path)
- **goVergeOS** - Go library (this project)

The original name "VergeOS Go SDK" used the term "SDK" which implies more maturity and comprehensiveness than appropriate for what is essentially a Go API client library.

This project serves as the foundation for the Terraform Provider, Prometheus Exporter, and the upcoming Cluster API (CAPI) provider.

**Evolution of Naming:**

1. **Original**: `vergeos-go-sdk` - SDK terminology was too formal
2. **v0.1.0-alpha**: `goVergeOS` - Brand consistency with mixed case
3. **v0.1.0**: `govergeos` - Go-idiomatic lowercase module path

The mixed-case `goVergeOS` import path caused friction:
- Go module proxy encodes uppercase as `go!verge!o!s` in URLs
- Users naturally type lowercase and get errors
- No major Go libraries use mixed-case module paths (the `github.com/Sirupsen/logrus` → `github.com/sirupsen/logrus` rename is a cautionary tale)

**Decision:** Use lowercase `govergeos` for all code paths (module path, repository name, User-Agent) while keeping "goVergeOS" as the marketing/branding name in documentation and titles.

**Rationale:**
- Go conventions strongly prefer lowercase import paths
- All major Go SDKs use lowercase: `aws-sdk-go`, `go-github`, `stripe-go`
- The package name `vergeos` is what developers actually type in code
- Brand name in docs ("goVergeOS") vs. import path (`govergeos`) is a common pattern
- Matches `pyvergeos` Python package naming at the module level

**Consequences:**
- Repository: `github.com/verge-io/govergeos`
- Module path: `github.com/verge-io/govergeos`
- Import: `import vergeos "github.com/verge-io/govergeos"`
- User-Agent: `govergeos/1.0`
- Documentation titles: "goVergeOS" (branding preserved)
- Package name: `vergeos` (unchanged, fully idiomatic)
- Users can now `go get github.com/verge-io/govergeos` without case sensitivity issues

---

## ADR-012: Service Interfaces for Mock Testing

**Date:** 2026-01-21

**Status:** Accepted

**Context:** The SDK currently implements all services as concrete structs with no corresponding interfaces. This makes it difficult for consumers (Terraform Provider, Prometheus Exporter, CAPI Provider) to:
- Write unit tests with mocked SDK calls
- Swap implementations for different behaviors
- Use dependency injection patterns

Currently, testing SDK consumers requires either:
- Spinning up a real VergeOS instance (integration testing only)
- HTTP-level mocking with `httptest`, which is brittle and couples tests to implementation details

**Decision:** Add interface definitions for all services to enable mock testing and dependency injection.

**Proposed Implementation:**
1. Define an interface for each service (e.g., `VMServiceInterface`, `NetworkServiceInterface`)
2. Update Client struct to use interface types instead of concrete pointers
3. Concrete service structs continue to implement the interfaces
4. Optionally provide a `mock` subpackage with test doubles or `mockgen` compatibility

**Rationale:**
- Enables consumers to write fast, isolated unit tests
- Follows Go best practices for testable library design
- Matches patterns in other infrastructure SDKs (AWS, GCP, Azure)
- Supports dependency injection for advanced use cases
- No breaking changes for existing consumers (interfaces are additive)

**Consequences:**
- Additional code to maintain (interface definitions mirror method signatures)
- Must keep interfaces in sync when adding/modifying service methods
- Enables significantly better testing experience for all SDK consumers
- Positions SDK as enterprise-ready with professional testing support

---

## ADR-013: API Schema Source and Type Mapping

**Date:** 2026-01-23

**Status:** Accepted

**Context:** The SDK initially used JSON schema files copied from `/usr/lib/appserver/schema/v4/` on a VergeOS system. Testing against live API revealed that these schema files describe *logical types* but not *runtime JSON serialization*. Key discrepancies discovered:

1. **`parent_system` fields**: Schema shows `$type: "row"` (integer) but API returns `"self"` (string)
2. **Nullable row fields**: Schema doesn't indicate when foreign key fields can be `null`
3. **Polymorphic IDs**: `$key` fields sometimes serialize as strings instead of integers

The internal API team clarified that filesystem schema files are pre-translation definitions and are **not authoritative**.

**Decision:**

1. **Schema Source**: Use runtime-extracted schema via `root-yb-api /v4 -f 'name,schema'` command as the authoritative source. Store in `.claude/reference/API-Schema/`.

2. **Type Mapping Rules**:
   - `parent_system` type fields → `string` (returns special values like `"self"`)
   - Optional `row` fields → `*int` or `FlexInt` (may be `null`)
   - Required `row` fields → `int` or `FlexInt`
   - All `$key` fields → `FlexInt` (handles string/int polymorphism)
   - `rows` type → omit from struct (one-to-many, not directly serialized)

3. **Verification Requirement**: Always test new field mappings against live API before committing.

**Rationale:**
- Runtime schema reflects actual API behavior after server-side translation
- Explicit type mapping rules prevent repeated discovery of the same issues
- `FlexInt` already exists in SDK (ADR-002) and handles ID polymorphism
- Documented exceptions prevent future developers from "fixing" correct behavior

**Consequences:**
- Schema in `.claude/reference/API-Schema/` is the single source of truth
- Deprecated schema folders (`api-schema-old-local/`, `dz/`) can be removed
- Type mapping exceptions documented in `ENDPOINTS.md` for quick reference
- Must verify against live API when schema `$type` doesn't match expected Go type

**References:**
- GitHub Issue: https://github.com/verge-io/goVergeOS/issues/2
- Related: ADR-002 (FlexInt Type for ID Handling)
- Documentation: `.claude/reference/API-Schema/ENDPOINTS.md`

---

## ADR-014: Volume String Keys

**Date:** 2026-01-23

**Status:** Accepted

**Context:** When implementing the Volumes service, testing against the live API revealed that volumes use SHA1 hash strings as their primary key (`$key`), not integers like other resources. The schema defines `"keyfield": "id"` where `id` is a 40-character SHA1 hash.

API Response example:
```json
[{"$key":"0d25c256a0c561c0b5bb9087f04fcb49f16a8048","id":"0d25c256a0c561c0b5bb9087f04fcb49f16a8048","name":"system-logs",...}]
```

**Decision:** The Volume type uses `string` for its Key field instead of `FlexInt`. All Volume service methods accept string IDs.

```go
type Volume struct {
    Key string `json:"$key,omitempty"`  // SHA1 hash, not FlexInt
    ID  string `json:"id,omitempty"`    // Same as Key for volumes
    // ...
}

func (s *VolumeService) Get(ctx context.Context, id string) (*Volume, error)
func (s *VolumeService) Update(ctx context.Context, id string, req *VolumeUpdateRequest) (*Volume, error)
func (s *VolumeService) Delete(ctx context.Context, id string) error
```

**Rationale:**
- Volumes are the first (and possibly only) resource type with string keys
- Using `string` directly is cleaner than extending `FlexInt` to handle this case
- The schema clearly indicates `id` is a 40-character SHA1 hash string
- Action endpoints (`volume_actions`) accept the string ID as the `volume` field

**Consequences:**
- Volume service has different method signatures than other services
- Must document this exception in interface comments
- Future resources with non-integer keys should follow this pattern
- Added `getStringKey()` helper function for extracting string keys from API responses

**Related:**
- ADR-002 (FlexInt Type for ID Handling) - contrast with integer key handling
- ADR-013 (API Schema Source) - verifying types against live API

---

## ADR-015: VNet Rule Enable/Disable Uses PUT Instead of Action Endpoint

**Date:** 2026-01-26

**Status:** Accepted

**Context:** The VNet Rules service initially implemented `Enable()` and `Disable()` methods that attempted to POST to a `/vnet_rule_actions` endpoint, following the pattern used by VMs (`/vm_actions`) and Networks (`/vnet_actions`). However, integration testing revealed this endpoint does not exist.

Analysis of the API schema showed:
1. The `vnet_rules.json` schema defines `enable` and `disable` actions within the table schema (lines 358-412)
2. No `vnet_rule_actions.json` endpoint file exists in the schema (45 other `*_actions.json` files exist)
3. The main `v4.json` schema has no `vnet_rule_actions` entry
4. The `enabled` field is a standard writable boolean field on the rule

This is an exception to the typical VergeOS API pattern where resources have dedicated `*_actions` endpoints for state-changing operations.

**Decision:** Implement `Enable()` and `Disable()` methods using PUT to update the `enabled` field directly, then optionally call `Networks.ApplyRules()` to apply the changes.

```go
func (s *VNetRuleService) Enable(ctx context.Context, id int, apply bool) error {
    return s.setEnabled(ctx, id, true, apply)
}

func (s *VNetRuleService) setEnabled(ctx context.Context, id int, enabled bool, apply bool) error {
    rule, err := s.Update(ctx, id, &VNetRuleUpdateRequest{Enabled: &enabled})
    if err != nil {
        return err
    }
    if apply && rule.VNet > 0 {
        return s.client.Networks.ApplyRules(ctx, int(rule.VNet))
    }
    return nil
}
```

**Rationale:**
- The `/vnet_rule_actions` endpoint does not exist in the VergeOS API
- Updating the `enabled` field via PUT is the standard RESTful approach
- The `Networks.ApplyRules()` method already exists and correctly calls `/vnet_actions` with action "apply"
- This approach composes existing, working functionality rather than inventing new endpoints
- Removed the `forceApply` parameter since the underlying `ApplyRules()` doesn't support it

**Consequences:**
- Breaking change: `Enable()` and `Disable()` signatures changed from `(id, apply, forceApply)` to `(id, apply)`
- Method now requires two API calls when `apply=true` (PUT + POST to vnet_actions)
- Follows RESTful conventions more closely than action-based approach
- Documents an API exception that future maintainers should be aware of

**Related:**
- ADR-013 (API Schema Source) - importance of verifying against live API
- `.claude/reference/API-Schema/ENDPOINTS.md` - documents this exception

---

## ADR-016: Mandatory Version Check in NewClient

**Date:** 2026-01-29

**Status:** Accepted

**Context:** The VergeOS API uses `/api/v4/` for all endpoints regardless of the actual software version. This means a server running VergeOS 4.x and one running 26.x both expose the same `/api/v4/` path, making it impossible to detect version mismatches from API paths alone.

The SDK targets VergeOS 26 and relies on features, fields, and behaviors specific to that version. Using the SDK against an older VergeOS installation may result in:
- Missing fields in API responses
- Endpoints that don't exist
- Different behavior for existing endpoints
- Confusing partial failures that are hard to diagnose

**Decision:** Perform a blocking version check in `NewClient()` that fetches `/version.json` and validates the server is running VergeOS 26.x. If the major version is not 26, client creation fails immediately with an `UnsupportedVersionError`.

```go
client, err := vergeos.NewClient(
    vergeos.WithBaseURL("https://host"),
    vergeos.WithCredentials("user", "pass"),
)
if err != nil {
    // "unsupported server version 4.2.0: this SDK requires VergeOS 26.x"
    log.Fatal(err)
}
```

**Alternatives Considered:**

1. **Lazy check on first API call** - Rejected because it delays error discovery; users might write significant code before hitting the check
2. **Optional check via `WithVersionCheck()` option** - Rejected because it leads to confusing partial failures when users forget to enable it
3. **No check at all** - Rejected because the SDK may silently malfunction on incompatible versions
4. **Warning instead of error** - Rejected because warnings are easily ignored and don't prevent the underlying compatibility issues

**Rationale:**
- **Fail fast** - Users get immediate, clear feedback if the server version is incompatible
- **No ambiguity** - The SDK either works fully or doesn't work at all; no partial compatibility
- **Simple implementation** - One check at startup, no caching or repeated checks needed
- **Clear error message** - Users know exactly what's wrong and what version is required
- **Choke point** - `NewClient()` is the natural place to validate prerequisites before any API calls

**Consequences:**
- `NewClient()` makes a network request to `/version.json` before returning
- Client creation fails if the server is not running VergeOS 26.x
- Users connecting to older VergeOS installations must use an older SDK version
- New `UnsupportedVersionError` type and `IsUnsupportedVersionError()` helper added
- Adds `getAbsolute()` internal method for fetching non-API paths

---

## ADR-017: Explicit Environment Configuration

**Date:** 2026-01-30

**Status:** Accepted

**Context:** The pyVergeOS SDK provides `VergeClient.from_env()` for convenient client creation from environment variables. The Go SDK currently requires manual `os.Getenv()` calls in every example and integration test. We need to add equivalent functionality while avoiding security and debugging pitfalls.

Three approaches were considered:

1. **`NewClientFromEnv()` function** - Separate constructor that reads environment variables
2. **Automatic env var fallback in `NewClient()`** - Check env vars when options not provided
3. **`WithEnvConfig()` option** - Explicit opt-in via functional option

**Decision:** Use `WithEnvConfig()` as an explicit opt-in option that can be composed with other options.

```go
// Simple: all config from environment
client, err := vergeos.NewClient(vergeos.WithEnvConfig())

// Advanced: env vars with explicit override
client, err := vergeos.NewClient(
    vergeos.WithEnvConfig(),
    vergeos.WithTimeout(60*time.Second),  // Override timeout from env
)
```

**Alternatives Rejected:**

1. **Automatic env var fallback** - Rejected because:
   - Implicit behavior makes debugging difficult ("why is it connecting to X?")
   - Security risk from accidental credential pickup
   - Test pollution when real credentials leak into test environment

2. **Separate `NewClientFromEnv()` function** - Rejected because:
   - Two entry points increases API surface
   - Cannot easily combine env vars with explicit overrides
   - Less composable than functional options pattern (ADR-003)

**Rationale:**
- **Explicit opt-in** - Environment variables are never automatically picked up, matching pyVergeOS behavior
- **Composable** - Works with existing functional options pattern; explicit options override env vars
- **Single entry point** - `NewClient()` remains the only constructor
- **Debuggable** - When `WithEnvConfig()` is in the code, the intent is clear
- **Secure by default** - No accidental credential pickup from environment

**Environment Variables Supported:**
| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `VERGEOS_HOST` | Yes | - | Base URL |
| `VERGEOS_USERNAME` | No* | - | Basic auth username |
| `VERGEOS_PASSWORD` | No* | - | Basic auth password |
| `VERGEOS_API_KEY` | No* | - | Bearer token auth |
| `VERGEOS_VERIFY_SSL` | No | `true` | TLS certificate verification |
| `VERGEOS_TIMEOUT` | No | `30` | Request timeout (seconds) |

*One of (USERNAME+PASSWORD) or API_KEY required.

**Consequences:**
- New `WithEnvConfig()` option function added to `client.go`
- Environment variables use `VERGEOS_` prefix (matching existing convention)
- Default SSL verification is `true` (secure by default, matching pyVergeOS)
- Integration tests can use `WithEnvConfig()` to reduce boilerplate
- Examples demonstrate both explicit options and env-based configuration

**Related:**
- ADR-003 (Functional Options Pattern) - `WithEnvConfig()` follows this pattern
- Plan: `.claude/plans/clientenvsslconfig.md`


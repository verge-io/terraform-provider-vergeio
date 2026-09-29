package acctest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/provider"
	"terraform-provider-vergeio/internal/provider/vergeio"
)

// ResourcePrefix is required on every object an acceptance test creates.
// Sweepers delete leftovers by this prefix and nothing else.
const ResourcePrefix = "tf-acc-"

const (
	envAccHost     = "TF_ACC_VERGEIO_HOST"
	envAccUsername = "TF_ACC_VERGEIO_USERNAME"
	envAccPassword = "TF_ACC_VERGEIO_PASSWORD"
)

// ProtoV6ProviderFactories instantiates the real provider for acceptance tests.
var ProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"vergeio": providerserver.NewProtocol6WithError(provider.New("test")()),
}

// Name returns a unique object name that starts with ResourcePrefix.
// suffix is a short resource kind such as "network" or "vm".
func Name(suffix string) string {
	suffix = strings.TrimSpace(suffix)
	suffix = strings.Trim(suffix, "-")
	suffix = strings.TrimPrefix(suffix, ResourcePrefix)
	if suffix == "" {
		suffix = "resource"
	}
	return ResourcePrefix + suffix + "-" + randomSuffix()
}

// RequirePrefix reports an error when name would be invisible to the sweeper.
func RequirePrefix(name string) error {
	if !HasPrefix(name) {
		return fmt.Errorf("acceptance object name %q must start with %q", name, ResourcePrefix)
	}
	return nil
}

// HasPrefix reports whether name belongs to the acceptance-test namespace.
func HasPrefix(name string) bool {
	return strings.HasPrefix(name, ResourcePrefix)
}

// CredentialsConfigured reports whether lab credentials are present.
// It does not report whether TF_ACC is set.
func CredentialsConfigured() bool {
	return os.Getenv(envAccHost) != "" &&
		os.Getenv(envAccUsername) != "" &&
		os.Getenv(envAccPassword) != ""
}

// PreCheck skips the test unless TF_ACC is set and lab credentials are present.
// Unit tests never need those variables.
func PreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless TF_ACC=1")
	}
	if !CredentialsConfigured() {
		t.Skipf("acceptance tests skipped: set %s, %s, and %s", envAccHost, envAccUsername, envAccPassword)
	}
	t.Logf("acceptance test will connect to VergeOS at %s (user: %s)", os.Getenv(envAccHost), os.Getenv(envAccUsername))
}

// ProviderConfig is the provider block for acceptance configurations.
// When credentials are absent the block is empty so PreCheck can skip
// before Terraform runs.
func ProviderConfig() string {
	host := os.Getenv(envAccHost)
	username := os.Getenv(envAccUsername)
	password := os.Getenv(envAccPassword)
	if host == "" || username == "" || password == "" {
		return `
provider "vergeio" {
  # Set TF_ACC_VERGEIO_HOST, TF_ACC_VERGEIO_USERNAME, and TF_ACC_VERGEIO_PASSWORD.
}
`
	}
	return fmt.Sprintf(`
provider "vergeio" {
  host     = %q
  username = %q
  password = %q
  insecure = true
}
`, host, username, password)
}

// Config joins the provider block with resource configuration.
func Config(resources string) string {
	return ProviderConfig() + "\n" + resources
}

// SDKClient is the govergeos client used by sweepers and destroy checks.
func SDKClient() (*vergeos.Client, error) {
	host := os.Getenv(envAccHost)
	username := os.Getenv(envAccUsername)
	password := os.Getenv(envAccPassword)
	if host == "" || username == "" || password == "" {
		return nil, fmt.Errorf("%s, %s, and %s are required", envAccHost, envAccUsername, envAccPassword)
	}
	return vergeos.NewClient(
		vergeos.WithBaseURL(vergeio.EnsureHTTPSPrefix(host)),
		vergeos.WithCredentials(username, password),
		vergeos.WithInsecureTLS(true),
	)
}

// CheckDeleted fails when a destroyed resource is still returned by get.
// get should return the govergeos not-found error when the object is gone.
func CheckDeleted(s *terraform.State, resourceType string, get func(context.Context, int) error) error {
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != resourceType {
			continue
		}
		if rs.Primary.ID == "" {
			continue
		}
		id, err := strconv.Atoi(rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("%s id %q: %w", resourceType, rs.Primary.ID, err)
		}
		err = get(ctx, id)
		if err == nil {
			return fmt.Errorf("%s %s still exists after destroy", resourceType, rs.Primary.ID)
		}
		if !vergeos.IsNotFoundError(err) {
			return fmt.Errorf("checking %s %s was destroyed: %w", resourceType, rs.Primary.ID, err)
		}
	}
	return nil
}

func randomSuffix() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(buf)
}

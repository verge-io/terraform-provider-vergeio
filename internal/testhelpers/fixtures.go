package testhelpers

// TestAccResourceConfig provides base configuration for resource testing
func TestAccResourceConfig() string {
	return ProviderConfig()
}

// TestAccDataSourceConfig provides base configuration for data source testing
func TestAccDataSourceConfig() string {
	return ProviderConfig()
}

// Test fixture data - these will be expanded based on specific resource needs
var (
	// TestNetworkName is a test network identifier
	TestNetworkName = "test-network"
	
	// TestVMName is a test VM identifier  
	TestVMName = "test-vm"
	
	// TestUserName is a test user identifier
	TestUserName = "test-user"
)

// GetTestConfig returns configuration for specific resource testing
func GetTestConfig(resourceType string) string {
	baseConfig := ProviderConfig()
	
	switch resourceType {
	case "network":
		return baseConfig + `
resource "vergeio_network" "test" {
  name = "` + TestNetworkName + `"
  # Additional network configuration will be added
}
`
	case "vm":
		return baseConfig + `
resource "vergeio_vm" "test" {
  name = "` + TestVMName + `"
  # Additional VM configuration will be added
}
`
	case "user":
		return baseConfig + `
resource "vergeio_user" "test" {
  username = "` + TestUserName + `"
  # Additional user configuration will be added
}
`
	default:
		return baseConfig
	}
}
// Package vergeos provides a Go client library for the VergeOS API.
//
// goVergeOS provides a convenient way to interact with VergeOS infrastructure
// from Go applications. It handles authentication, request building, and
// response parsing.
//
// # Quick Start
//
// Create a client and start making API calls:
//
//	client, err := vergeos.NewClient(
//	    vergeos.WithBaseURL("https://your-vergeos-host"),
//	    vergeos.WithCredentials("username", "password"),
//	    vergeos.WithInsecureTLS(true),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// List all VMs
//	vms, err := client.VMs.List(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Create a VM
//	vm, err := client.VMs.Create(ctx, &vergeos.VMCreateRequest{
//	    Name:     "my-vm",
//	    CPUCores: 4,
//	    RAM:      8192,
//	})
//
// # Authentication
//
// The library uses HTTP Basic Authentication. You can use either a "Normal" user
// or an "API" user account. MFA must be disabled for the user account.
//
// # Query Options
//
// List operations support filtering, sorting, and pagination:
//
//	vms, err := client.VMs.List(ctx,
//	    vergeos.WithFilter("enabled eq true"),
//	    vergeos.WithSort("name"),
//	    vergeos.WithLimit(50),
//	)
//
// # Error Handling
//
// The library provides typed errors and helper functions:
//
//	vm, err := client.VMs.Get(ctx, vmID)
//	if vergeos.IsNotFoundError(err) {
//	    // Handle not found
//	}
//	if vergeos.IsAuthError(err) {
//	    // Handle authentication failure
//	}
package vergeos

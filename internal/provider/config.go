// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package provider

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Environment variable names shared with the Ansible collection and govergeos.
const (
	envHost      = "VERGEOS_HOST"
	envUsername  = "VERGEOS_USERNAME"
	envPassword  = "VERGEOS_PASSWORD"
	envAPIKey    = "VERGEOS_API_KEY"
	envInsecure  = "VERGEOS_INSECURE"
	envVerifySSL = "VERGEOS_VERIFY_SSL"
	envTimeout   = "VERGEOS_TIMEOUT"
)

// resolveProviderConfig merges provider configuration with the environment.
// A value in the provider block wins over the environment variable for the
// same setting. Authentication is an API key, or a username and password.
// When both are set, the API key is used.
func resolveProviderConfig(data vergeioProviderModel) (vergeio.ClientConfig, diag.Diagnostics) {
	var diags diag.Diagnostics

	if unknown := unknownProviderAttributes(data); len(unknown) > 0 {
		names := joinAnd(unknown)
		diags.AddError(
			"Unknown provider configuration: "+names,
			"The provider cannot connect while these values are unknown: "+names+
				". Apply the source of each value first, or set it in the provider block or the matching VERGEOS_* environment variable.",
		)
		return vergeio.ClientConfig{}, diags
	}

	host := configuredString(data.Host, envHost)
	username := configuredString(data.Username, envUsername)
	password := configuredString(data.Password, envPassword)
	apiKey := configuredString(data.APIKey, envAPIKey)

	insecure, insecureDiags := configuredInsecure(data.Insecure)
	diags.Append(insecureDiags...)

	timeout, timeoutDiags := configuredTimeout(data.Timeout)
	diags.Append(timeoutDiags...)

	if missing := missingProviderSettings(host, username, password, apiKey); len(missing) > 0 {
		names := strings.Join(missing, "; ")
		diags.AddError(
			"Missing required provider configuration: "+names,
			"Set each missing value in the provider block or with its environment variable ("+
				envHost+", "+envUsername+", "+envPassword+", "+envAPIKey+
				"). Authentication requires an API key or both a username and password. When both are set, the API key is used.",
		)
	}

	if diags.HasError() {
		return vergeio.ClientConfig{}, diags
	}

	return vergeio.ClientConfig{
		Host:     host,
		Username: username,
		Password: password,
		APIKey:   apiKey,
		Insecure: insecure,
		Timeout:  timeout,
	}, diags
}

func unknownProviderAttributes(data vergeioProviderModel) []string {
	var names []string
	if data.Host.IsUnknown() {
		names = append(names, "host")
	}
	if data.Username.IsUnknown() {
		names = append(names, "username")
	}
	if data.Password.IsUnknown() {
		names = append(names, "password")
	}
	if data.APIKey.IsUnknown() {
		names = append(names, "api_key")
	}
	if data.Insecure.IsUnknown() {
		names = append(names, "insecure")
	}
	if data.Timeout.IsUnknown() {
		names = append(names, "timeout")
	}
	return names
}

// missingProviderSettings names the settings Configure still needs.
// An API key satisfies authentication on its own. Otherwise both username
// and password are required.
func missingProviderSettings(host, username, password, apiKey string) []string {
	var missing []string
	if strings.TrimSpace(host) == "" {
		missing = append(missing, "host")
	}
	switch {
	case strings.TrimSpace(apiKey) != "":
	case strings.TrimSpace(username) == "" && strings.TrimSpace(password) == "":
		missing = append(missing, "api_key or username and password")
	case strings.TrimSpace(username) == "":
		missing = append(missing, "username or api_key")
	case strings.TrimSpace(password) == "":
		missing = append(missing, "password or api_key")
	}
	return missing
}

// configuredString returns the provider value when it is set, including an
// explicit empty string. Otherwise it returns the trimmed environment variable.
func configuredString(value types.String, envName string) string {
	if !value.IsNull() && !value.IsUnknown() {
		return value.ValueString()
	}
	return strings.TrimSpace(os.Getenv(envName))
}

// configuredInsecure returns whether to skip TLS verification.
// The provider block wins. Otherwise VERGEOS_INSECURE is used, then
// VERGEOS_VERIFY_SSL where false means skip verification. The two environment
// variables must agree when both are set.
func configuredInsecure(value types.Bool) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !value.IsNull() && !value.IsUnknown() {
		return value.ValueBool(), nil
	}

	insecureRaw, insecureOK := lookupEnv(envInsecure)
	verifyRaw, verifyOK := lookupEnv(envVerifySSL)

	insecure, insecureParsed := false, false
	if insecureOK {
		parsed, err := parseEnvBool(insecureRaw)
		if err != nil {
			diags.AddError(
				"Invalid provider configuration: "+envInsecure,
				fmt.Sprintf("%s must be true or false, got %q.", envInsecure, insecureRaw),
			)
		} else {
			insecure = parsed
			insecureParsed = true
		}
	}
	if verifyOK {
		parsed, err := parseEnvBool(verifyRaw)
		if err != nil {
			diags.AddError(
				"Invalid provider configuration: "+envVerifySSL,
				fmt.Sprintf("%s must be true or false, got %q.", envVerifySSL, verifyRaw),
			)
		} else if insecureParsed && insecure != !parsed {
			diags.AddError(
				"Invalid provider configuration: "+envInsecure+" and "+envVerifySSL,
				fmt.Sprintf(
					"%s=%q and %s=%q disagree. True for %s and false for %s both skip TLS certificate verification. Unset one of them, or set insecure in the provider block.",
					envInsecure, insecureRaw, envVerifySSL, verifyRaw, envInsecure, envVerifySSL,
				),
			)
		} else if !insecureParsed {
			insecure = !parsed
		}
	}
	return insecure, diags
}

// configuredTimeout returns the HTTP timeout. The provider block wins over
// VERGEOS_TIMEOUT. Both are a positive number of seconds. An unset timeout
// uses the provider default.
func configuredTimeout(value types.Int64) (time.Duration, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !value.IsNull() && !value.IsUnknown() {
		seconds := value.ValueInt64()
		if seconds <= 0 {
			diags.AddError(
				"Invalid provider configuration: timeout",
				fmt.Sprintf("timeout must be a positive number of seconds, got %d.", seconds),
			)
			return 0, diags
		}
		return time.Duration(seconds) * time.Second, nil
	}

	raw, ok := lookupEnv(envTimeout)
	if !ok {
		return vergeio.DefaultTimeout, nil
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		diags.AddError(
			"Invalid provider configuration: "+envTimeout,
			fmt.Sprintf("%s must be a positive number of seconds, got %q.", envTimeout, raw),
		)
		return 0, diags
	}
	return time.Duration(seconds) * time.Second, nil
}

func lookupEnv(name string) (string, bool) {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return "", false
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	return raw, true
}

func parseEnvBool(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "yes", "y", "on":
		return true, nil
	case "no", "n", "off":
		return false, nil
	}
	return strconv.ParseBool(value)
}

func joinAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
	}
}

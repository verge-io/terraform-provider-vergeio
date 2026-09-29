// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"encoding/json"
	"strings"
)

const redactedSecret = "***"

// maskSecretPayload returns payload with secret JSON values replaced.
// The original payload is left unchanged. Non-JSON bodies are returned as-is.
func maskSecretPayload(payload string) string {
	if strings.TrimSpace(payload) == "" {
		return payload
	}

	var decoded any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		return payload
	}

	encoded, err := json.Marshal(maskSecretJSON(decoded))
	if err != nil {
		return payload
	}
	return string(encoded)
}

func maskSecretJSON(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for key, child := range val {
			if secretJSONKey(key) {
				out[key] = redactedSecret
				continue
			}
			out[key] = maskSecretJSON(child)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, child := range val {
			out[i] = maskSecretJSON(child)
		}
		return out
	default:
		return v
	}
}

// secretJSONKey reports whether a JSON field holds a secret.
// change_password and console_pass_enabled are flags, not secret values.
func secretJSONKey(key string) bool {
	k := strings.ToLower(key)
	switch k {
	case "change_password", "console_pass_enabled":
		return false
	}
	if k == "password" || strings.Contains(k, "password") ||
		k == "console_pass" ||
		k == "passwd" || k == "passphrase" ||
		strings.Contains(k, "secret") ||
		k == "token" || strings.HasSuffix(k, "_token") ||
		strings.Contains(k, "private_key") ||
		strings.Contains(k, "api_key") || k == "apikey" {
		return true
	}
	return false
}

// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

const (
	// clientSecretSettingsKey is the write-only key inside an auth source
	// settings document. govergeos strips it from responses. It must be sent
	// through client_secret_wo so it is not stored in Terraform state.
	clientSecretSettingsKey = "client_secret"
	// settingsDebugKey is injected by VergeOS into the stored settings
	// document. It mirrors the debug column. Tracking it would plan a change
	// on every refresh.
	settingsDebugKey = "debug"
)

// settingsCanonicalModifier rewrites settings to one JSON encoding and
// rejects a client_secret key. The plan value is what later applies compare
// with state, so key order and spacing cannot show up as drift.
type settingsCanonicalModifier struct{}

func (m settingsCanonicalModifier) Description(context.Context) string {
	return "Canonicalizes the auth source settings JSON and rejects client_secret."
}

func (m settingsCanonicalModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m settingsCanonicalModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}
	text, err := canonicalSettings(req.PlanValue.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid auth source settings", err.Error())
		return
	}
	resp.PlanValue = types.StringValue(text)
}

// canonicalSettings parses a settings document and encodes it with sorted
// keys and no extra whitespace. client_secret is rejected.
func canonicalSettings(raw string) (string, error) {
	parsed, err := parseSettings(raw)
	if err != nil {
		return "", err
	}
	return marshalSettings(parsed)
}

// parseSettings decodes a JSON object. A client_secret key is an error so
// the secret cannot be planned or stored. The error text does not include
// the document.
func parseSettings(raw string) (map[string]any, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, fmt.Errorf("settings must be a JSON object")
	}
	if dec.More() {
		return nil, fmt.Errorf("settings must be a JSON object")
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("settings must be a JSON object")
	}
	if _, ok := obj[clientSecretSettingsKey]; ok {
		return nil, fmt.Errorf("settings must not include client_secret; set client_secret_wo and client_secret_wo_version")
	}
	return obj, nil
}

// reconcileSettings keeps the keys Terraform already tracks and refreshes
// their values from VergeOS. Keys the server added, including debug and
// client_secret, stay out of state. A null tracked document stays null so an
// omitted settings argument does not adopt the stored document.
func reconcileSettings(tracked types.String, server vergeos.AuthSourceSettings) (types.String, error) {
	if tracked.IsNull() || tracked.IsUnknown() {
		return types.StringNull(), nil
	}
	parsed, err := parseSettings(tracked.ValueString())
	if err != nil {
		return types.StringNull(), err
	}
	kept := make(map[string]any, len(parsed))
	for key := range parsed {
		if key == clientSecretSettingsKey || key == settingsDebugKey {
			continue
		}
		value, ok := server[key]
		if !ok {
			continue
		}
		kept[key] = value
	}
	text, err := marshalSettings(kept)
	if err != nil {
		return types.StringNull(), err
	}
	return types.StringValue(text), nil
}

// settingsDocument is the settings object to send. secret is added as
// client_secret when the write-only value should be sent. A nil result
// leaves the stored document unchanged. govergeos merges a non-nil document
// with the stored keys, so client_secret and keys omitted here are kept.
func settingsDocument(raw types.String, secret string) (vergeos.AuthSourceSettings, error) {
	doc := vergeos.AuthSourceSettings{}
	if !raw.IsNull() && !raw.IsUnknown() && strings.TrimSpace(raw.ValueString()) != "" {
		parsed, err := parseSettings(raw.ValueString())
		if err != nil {
			return nil, err
		}
		for key, value := range parsed {
			if key == settingsDebugKey {
				continue
			}
			doc[key] = value
		}
	}
	if secret != "" {
		doc[clientSecretSettingsKey] = vergeos.NewWriteOnlySecret(secret)
	}
	if len(doc) == 0 {
		return nil, nil
	}
	return doc, nil
}

// writeOnlySecret is the client secret to send. Create always sends a known
// value. Update sends it only when client_secret_wo_version changed, because
// the write-only value is null in the plan and a change to it alone does
// not call Update.
func writeOnlySecret(planVersion, stateVersion types.Int64, config types.String, hasState bool) string {
	if config.IsNull() || config.IsUnknown() {
		return ""
	}
	if hasState && planVersion.Equal(stateVersion) {
		return ""
	}
	return config.ValueString()
}

func marshalSettings(doc map[string]any) (string, error) {
	text, err := marshalJSON(normalizeJSON(doc))
	if err != nil {
		return "", fmt.Errorf("settings must be a JSON object")
	}
	return string(text), nil
}

// orderedJSON marshals with sorted keys. encoding/json map order is random,
// and a refresh would otherwise plan a settings change that did not happen.
type orderedJSON []jsonField

type jsonField struct {
	Key string
	Val any
}

func (o orderedJSON) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	if err := buf.WriteByte('{'); err != nil {
		return nil, err
	}
	for i, field := range o {
		if i > 0 {
			if err := buf.WriteByte(','); err != nil {
				return nil, err
			}
		}
		key, err := marshalJSON(field.Key)
		if err != nil {
			return nil, err
		}
		if _, err := buf.Write(key); err != nil {
			return nil, err
		}
		if err := buf.WriteByte(':'); err != nil {
			return nil, err
		}
		val, err := marshalJSON(field.Val)
		if err != nil {
			return nil, err
		}
		if _, err := buf.Write(val); err != nil {
			return nil, err
		}
	}
	if err := buf.WriteByte('}'); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func normalizeJSON(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		obj := make(orderedJSON, 0, len(keys))
		for _, key := range keys {
			obj = append(obj, jsonField{Key: key, Val: normalizeJSON(typed[key])})
		}
		return obj
	case []any:
		items := make([]any, len(typed))
		for i, item := range typed {
			items[i] = normalizeJSON(item)
		}
		return items
	default:
		return v
	}
}

func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}

// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const (
	catalogEndpoint            = APIEndpoint + "/catalogs"
	catalogRepositoryEndpoint  = APIEndpoint + "/catalog_repositories"
	tenantRecipeEndpoint       = APIEndpoint + "/tenant_recipes"
	recipeQuestionEndpoint     = APIEndpoint + "/recipe_questions"
	tenantRecipeActionEndpoint = APIEndpoint + "/tenant_recipe_actions"
)

// CatalogCreate is the POST body for a catalog.
// govergeos Catalogs has no Create method.
type CatalogCreate struct {
	Name            string `json:"name"`
	Repository      int    `json:"repository"`
	Description     string `json:"description,omitempty"`
	PublishingScope string `json:"publishing_scope,omitempty"`
	Enabled         bool   `json:"enabled"`
}

// TenantRecipeCreate is the POST body for a tenant recipe.
// govergeos TenantRecipes has no Create method. Catalog is the catalog hex key.
// tenant_snapshot is omitted. VergeOS sets that tenants-table row itself and
// returns 422 if a client sends it, the same way vm_recipes.vm_snapshot is read-only.
type TenantRecipeCreate struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Catalog     string `json:"catalog"`
	Tenant      int    `json:"tenant"`
	Version     string `json:"version,omitempty"`
}

// CreateCatalog posts a catalog into a repository and returns its hex key.
func (c *Client) CreateCatalog(ctx context.Context, req CatalogCreate) (string, error) {
	return c.postKey(ctx, catalogEndpoint, req)
}

// DeleteCatalog removes one catalog. A missing catalog is success.
func (c *Client) DeleteCatalog(ctx context.Context, key string) error {
	return c.deleteHex(ctx, catalogEndpoint, key)
}

// CreateTenantRecipe posts a tenant recipe for a tenant and returns its hex key.
// VergeOS copies that tenant into tenant_snapshot. The caller does not.
func (c *Client) CreateTenantRecipe(ctx context.Context, req TenantRecipeCreate) (string, error) {
	if strings.TrimSpace(req.Version) == "" {
		req.Version = "1.0.0"
	}
	return c.postKey(ctx, tenantRecipeEndpoint, req)
}

// DeleteTenantRecipe removes one tenant recipe. A missing recipe is success.
// VergeOS refuses the delete while a recipe instance still points at it.
func (c *Client) DeleteTenantRecipe(ctx context.Context, key string) error {
	return c.deleteHex(ctx, tenantRecipeEndpoint, key)
}

// DisableRecipeQuestion turns one question off so deploy does not require an answer.
func (c *Client) DisableRecipeQuestion(ctx context.Context, id int) error {
	if c == nil {
		return fmt.Errorf("vergeio client is nil")
	}
	if id <= 0 {
		return fmt.Errorf("recipe question id %d is not a positive integer", id)
	}
	payload, err := json.Marshal(map[string]bool{"enabled": false})
	if err != nil {
		return err
	}
	resp, err := c.Put(ctx, ObjectPath(recipeQuestionEndpoint, fmt.Sprintf("%d", id)), bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	return discardResponse(resp)
}

// RepublishTenantRecipe posts the republish action after a question change.
func (c *Client) RepublishTenantRecipe(ctx context.Context, key string) error {
	if c == nil {
		return fmt.Errorf("vergeio client is nil")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("recipe id is empty")
	}
	payload, err := json.Marshal(map[string]string{
		"tenant_recipe": key,
		"action":        "republish",
	})
	if err != nil {
		return err
	}
	resp, err := c.Post(ctx, tenantRecipeActionEndpoint, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	return discardResponse(resp)
}

// LocalCatalogRepositoryID returns the key of the repository named Local,
// or the first repository whose type is local.
func (c *Client) LocalCatalogRepositoryID(ctx context.Context) (int, error) {
	if c == nil {
		return 0, fmt.Errorf("vergeio client is nil")
	}
	rows, err := c.listCatalogRepositories(ctx, fmt.Sprintf("name eq '%s'", EscapeFilterValue("Local")))
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		rows, err = c.listCatalogRepositories(ctx, "")
		if err != nil {
			return 0, err
		}
	}
	for _, row := range rows {
		if strings.EqualFold(row.Name, "Local") {
			return row.Key, nil
		}
	}
	for _, row := range rows {
		if strings.EqualFold(row.Type, "local") {
			return row.Key, nil
		}
	}
	return 0, fmt.Errorf("no local catalog repository")
}

type catalogRepositoryRow struct {
	Key  int
	Name string
	Type string
}

func (c *Client) listCatalogRepositories(ctx context.Context, filter string) ([]catalogRepositoryRow, error) {
	resp, err := c.Get(ctx, catalogRepositoryEndpoint, &Options{
		Fields: "$key,name,type",
		Filter: filter,
	})
	if err != nil {
		return nil, err
	}
	body, err := readResponse(resp)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, nil
	}
	var rawRows []struct {
		Key  json.RawMessage `json:"$key"`
		Name string          `json:"name"`
		Type string          `json:"type"`
	}
	if err := json.Unmarshal(body, &rawRows); err != nil {
		return nil, fmt.Errorf("catalog repositories: %w", err)
	}
	rows := make([]catalogRepositoryRow, 0, len(rawRows))
	for _, raw := range rawRows {
		key, err := parsePositiveID(raw.Key)
		if err != nil {
			return nil, fmt.Errorf("catalog repository %s: %w", raw.Name, err)
		}
		rows = append(rows, catalogRepositoryRow{Key: key, Name: raw.Name, Type: raw.Type})
	}
	return rows, nil
}

func parsePositiveID(raw json.RawMessage) (int, error) {
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if text == "" || text == "null" {
		return 0, fmt.Errorf("missing id")
	}
	n, err := strconv.Atoi(text)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("id %s is not a positive integer", text)
	}
	return n, nil
}

func (c *Client) postKey(ctx context.Context, endpoint string, payload any) (string, error) {
	if c == nil {
		return "", fmt.Errorf("vergeio client is nil")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	resp, err := c.Post(ctx, endpoint, bytes.NewBuffer(raw))
	if err != nil {
		return "", err
	}
	body, err := readResponse(resp)
	if err != nil {
		return "", err
	}
	var decoded struct {
		Key json.RawMessage `json:"$key"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("create %s: %w", endpoint, err)
	}
	key := strings.Trim(string(decoded.Key), `"`)
	if key == "" || key == "null" {
		return "", fmt.Errorf("create %s returned no $key: %s", endpoint, body)
	}
	return key, nil
}

func (c *Client) deleteHex(ctx context.Context, endpoint, key string) error {
	if c == nil {
		return fmt.Errorf("vergeio client is nil")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("id is empty")
	}
	resp, err := c.Delete(ctx, ObjectPath(endpoint, key))
	if err != nil {
		var apiErr Error
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil
		}
		return err
	}
	return discardResponse(resp)
}

func readResponse(resp *http.Response) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, nil
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(resp.Body)
}

func discardResponse(resp *http.Response) error {
	_, err := readResponse(resp)
	return err
}

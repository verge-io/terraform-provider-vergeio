// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// DownloadVMRecipe asks VergeOS to download a catalog recipe so it can be
// deployed. The request is PUT vm_recipes/{key}?action=download. A recipe
// that is already on the node may still return an error. The caller checks
// the downloaded flag.
func (c *Client) DownloadVMRecipe(ctx context.Context, key string) error {
	if c == nil {
		return fmt.Errorf("vergeio client is nil")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("recipe id is empty")
	}
	resp, err := c.Put(ctx, ObjectPath(VMRecipeEndpoint, key)+"?action=download", bytes.NewBufferString("{}"))
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
	}
	return err
}

// DeleteVMRecipeInstance removes one recipe instance row.
// govergeos VMRecipeInstances has no Delete method.
// A missing row is success.
func (c *Client) DeleteVMRecipeInstance(ctx context.Context, id int) error {
	return c.deleteRecipeInstance(ctx, RecipeInstanceEndpoint, id)
}

// DeleteTenantRecipeInstance removes one tenant recipe instance row.
// govergeos TenantRecipeInstances has no Delete method.
// A missing row is success.
func (c *Client) DeleteTenantRecipeInstance(ctx context.Context, id int) error {
	return c.deleteRecipeInstance(ctx, TenantRecipeInstanceEndpoint, id)
}

func (c *Client) deleteRecipeInstance(ctx context.Context, endpoint string, id int) error {
	if c == nil {
		return fmt.Errorf("vergeio client is nil")
	}
	if id <= 0 {
		return fmt.Errorf("recipe instance id %d is not a positive integer", id)
	}
	resp, err := c.Delete(ctx, ObjectPath(endpoint, strconv.Itoa(id)))
	if err != nil {
		var apiErr Error
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil
		}
		return err
	}
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
	}
	return nil
}

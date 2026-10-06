// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// DeleteVMRecipeInstance removes one recipe instance row.
// govergeos VMRecipeInstances has no Delete method.
// A missing row is success.
func (c *Client) DeleteVMRecipeInstance(ctx context.Context, id int) error {
	if c == nil {
		return fmt.Errorf("vergeio client is nil")
	}
	if id <= 0 {
		return fmt.Errorf("recipe instance id %d is not a positive integer", id)
	}
	resp, err := c.Delete(ctx, ObjectPath(RecipeInstanceEndpoint, strconv.Itoa(id)))
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

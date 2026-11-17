// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestListResourcesMatchManagedTypes(t *testing.T) {
	p := New("test")()
	withList, ok := p.(provider.ProviderWithListResources)
	if !ok {
		t.Fatal("provider does not implement list resources")
	}
	ctx := context.Background()
	got := map[string]bool{}
	for _, newList := range withList.ListResources(ctx) {
		lister := newList()
		resp := &resource.MetadataResponse{}
		lister.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "vergeio"}, resp)
		got[resp.TypeName] = true
	}
	for _, name := range []string{
		"vergeio_vm",
		"vergeio_network",
		"vergeio_tenant",
		"vergeio_user",
		"vergeio_group",
		"vergeio_tag",
		"vergeio_snapshot_profile",
	} {
		if !got[name] {
			t.Errorf("missing list resource %s", name)
		}
	}
	if len(got) != 7 {
		t.Fatalf("list resources = %#v", got)
	}
}

package vergeio

import (
	"context"
	"encoding/json"
	"io/ioutil"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Version structure to store version specific data in
type Version struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
	Hash    string `json:"hash,omitempty"`
}

func dataSourceVersionRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics
	resp, err := c.Get("version.json", nil)
	if err != nil {
		return diag.FromErr(err)
	}
	if resp != nil {
		if resp.StatusCode == 200 {
			var ver Version
			body, readerr := ioutil.ReadAll(resp.Body)
			if readerr != nil {
				return diag.FromErr(readerr)
			}
			decodeerr := json.Unmarshal(body, &ver)
			if decodeerr != nil {
				return diag.FromErr(decodeerr)
			}
			d.Set("hash", ver.Hash)
			d.Set("version", ver.Version)
			d.Set("name", ver.Name)
			d.SetId("version")
		}
	} else {
		return diag.Errorf("Error retrieving version")
	}
	return diags
}

func dataSourceVersion() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceVersionRead,
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"version": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"hash": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

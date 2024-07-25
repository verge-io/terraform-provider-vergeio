package vergeio

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Networks structure to store version specific data in
type Networks struct {
	ID          int    `json:"$key,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// NetworksEndpoint is the api endpoint representing this resource
const NetworksEndpoint = "api/v4/vnets"

func dataSourceNetworksRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	opts := Options{Fields: "$key,name,description"}
	if fn := d.Get("filter_name"); fn != nil && fn != "" {
		opts.Filter = fmt.Sprintf("name eq '%s'", fn.(string))
	}
	resp, err := c.Get(NetworksEndpoint, &opts)
	if err != nil {
		return diag.FromErr(err)
	}
	if resp != nil {
		if resp.StatusCode == 200 {
			var networkData []Networks
			body, readerr := ioutil.ReadAll(resp.Body)
			if readerr != nil {
				return diag.FromErr(readerr)
			}

			decodeerr := json.Unmarshal(body, &networkData)
			if decodeerr != nil {
				return diag.FromErr(decodeerr)
			}
			var networks []map[string]interface{}

			for _, network := range networkData {

				n := map[string]interface{}{
					"id":          network.ID,
					"name":        network.Name,
					"description": network.Description,
				}
				networks = append(networks, n)
			}
			err = d.Set("networks", networks)
			if err != nil {
				return diag.FromErr(err)
			}
			d.SetId(time.Now().UTC().Format(time.RFC3339Nano))
		}
	} else {
		return diag.Errorf("Error retrieving networks")
	}
	return diags
}

func dataSourceNetworks() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceNetworksRead,
		Schema: map[string]*schema.Schema{
			"filter_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: `If specified, results will be filtered to name`,
			},
			"networks": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeInt,
							Computed: true,
						},
						"name": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"description": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
		},
	}
}

package vergeio

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// AuthSources structure to store version specific data in
type AuthSources struct {
	ID     int    `json:"$key,omitempty"`
	Name   string `json:"name,omitempty"`
	Driver string `json:"driver,omitempty"`
}

// AuthSourcesEndpoint is the api endpoint representing this resource
const AuthSourcesEndpoint = "auth_sources.json"

func dataSourceAuthSourcesRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	resp, err := c.Get(AuthSourcesEndpoint, nil)
	if err != nil {
		return diag.FromErr(err)
	}
	if resp != nil {
		if resp.StatusCode == 200 {
			var authSourcesData []AuthSources
			body, readerr := ioutil.ReadAll(resp.Body)
			if readerr != nil {
				return diag.FromErr(readerr)
			}

			decodeerr := json.Unmarshal(body, &authSourcesData)
			if decodeerr != nil {
				return diag.FromErr(decodeerr)
			}
			var authSources []map[string]interface{}
			filterName, filterNameSet := d.GetOk("filter_name")
			filterDriver, filterDriverSet := d.GetOk("filter_driver")

			for _, authSource := range authSourcesData {

				n := map[string]interface{}{
					"id":     authSource.ID,
					"name":   authSource.Name,
					"driver": authSource.Driver,
				}
				if filterNameSet {
					if filterName == authSource.Name {
						authSources = append(authSources, n)
					}
				} else if filterDriverSet {
					if filterDriver == authSource.Driver {
						authSources = append(authSources, n)
					}
				} else {
					authSources = append(authSources, n)
				}
			}
			err = d.Set("authsources", authSources)
			if err != nil {
				return diag.FromErr(err)
			}
			d.SetId(time.Now().UTC().Format(time.RFC3339Nano))
		}
	} else {
		return diag.Errorf("Error retrieving authorization sources")
	}
	return diags
}

func dataSourceAuthSources() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceAuthSourcesRead,
		Schema: map[string]*schema.Schema{
			"filter_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: `If specified, results will be filtered to name`,
			},
			"filter_driver": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: `If specified, results will be filtered to driver`,
			},
			"authsources": {
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
						"driver": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
		},
	}
}

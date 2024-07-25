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

// Groups structure to store version specific data in
type Groups struct {
	ID          int    `json:"$key,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
}

// GroupsEndpoint is the api endpoint representing this resource
const GroupsEndpoint = "api/v4/groups"

func dataSourceGroupsRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	opts := Options{Fields: "$key,name,description,enabled"}
	if fn := d.Get("filter_name"); fn != nil && fn != "" {
		opts.Filter = fmt.Sprintf("name eq '%s'", fn.(string))
	}

	resp, err := c.Get(GroupsEndpoint, &opts)
	if err != nil {
		return diag.FromErr(err)
	}
	if resp != nil {
		if resp.StatusCode == 200 {
			var groupsData []Groups
			body, readerr := ioutil.ReadAll(resp.Body)
			if readerr != nil {
				return diag.FromErr(readerr)
			}

			decodeerr := json.Unmarshal(body, &groupsData)
			if decodeerr != nil {
				return diag.FromErr(decodeerr)
			}
			var groups []map[string]interface{}

			for _, group := range groupsData {

				n := map[string]interface{}{
					"id":          group.ID,
					"name":        group.Name,
					"description": group.Description,
					"enabled":     group.Enabled,
				}
				groups = append(groups, n)
			}
			err = d.Set("groups", groups)
			if err != nil {
				return diag.FromErr(err)
			}
			d.SetId(time.Now().UTC().Format(time.RFC3339Nano))
		}
	} else {
		return diag.Errorf("Error retrieving media sources")
	}
	return diags
}

func dataSourceGroups() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceGroupsRead,
		Schema: map[string]*schema.Schema{
			"filter_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: `If specified, results will be filtered to name`,
			},
			"groups": {
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
						"enabled": {
							Type:     schema.TypeBool,
							Computed: true,
						},
					},
				},
			},
		},
	}
}

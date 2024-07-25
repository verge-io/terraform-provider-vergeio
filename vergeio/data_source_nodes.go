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

// Nodes structure to store version specific data in
type Nodes struct {
	ID          int    `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// NodesEndpoint is the api endpoint representing this resource
const NodesEndpoint = "api/v4/nodes"

func dataSourceNodesRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	opts := Options{Fields: "id,name,description"}
	if fn := d.Get("filter_name"); fn != nil && fn != "" {
		opts.Filter = fmt.Sprintf("name eq '%s'", fn.(string))
	}
	resp, err := c.Get(NodesEndpoint, &opts)
	if err != nil {
		return diag.FromErr(err)
	}
	if resp != nil {
		if resp.StatusCode == 200 {
			var nodeData []Nodes
			body, readerr := ioutil.ReadAll(resp.Body)
			if readerr != nil {
				return diag.FromErr(readerr)
			}

			decodeerr := json.Unmarshal(body, &nodeData)
			if decodeerr != nil {
				return diag.FromErr(decodeerr)
			}
			var nodes []map[string]interface{}

			for _, node := range nodeData {

				n := map[string]interface{}{
					"id":          node.ID,
					"name":        node.Name,
					"description": node.Description,
				}
				nodes = append(nodes, n)
			}
			err = d.Set("nodes", nodes)
			if err != nil {
				return diag.FromErr(err)
			}
			d.SetId(time.Now().UTC().Format(time.RFC3339Nano))
		}
	} else {
		return diag.Errorf("Error retrieving nodes")
	}
	return diags
}

func dataSourceNodes() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceNodesRead,
		Schema: map[string]*schema.Schema{
			"filter_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: `If specified, results will be filtered to name`,
			},
			"nodes": {
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

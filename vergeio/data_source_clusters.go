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

// Clusters structure to store version specific data in
type Clusters struct {
	ID          int    `json:"$key,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// ClustersEndpoint is the api endpoint representing this resource
const ClustersEndpoint = "api/v4/clusters"

func dataSourceClustersRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	opts := Options{Fields: "$key,name,description"}
	if fn := d.Get("filter_name"); fn != nil && fn != "" {
		opts.Filter = fmt.Sprintf("name eq '%s'", fn.(string))
	}
	resp, err := c.Get(ClustersEndpoint, &opts)
	if err != nil {
		return diag.FromErr(err)
	}
	if resp != nil {
		if resp.StatusCode == 200 {
			var clusterData []Clusters
			body, readerr := ioutil.ReadAll(resp.Body)
			if readerr != nil {
				return diag.FromErr(readerr)
			}

			decodeerr := json.Unmarshal(body, &clusterData)
			if decodeerr != nil {
				return diag.FromErr(decodeerr)
			}
			var clusters []map[string]interface{}

			for _, cluster := range clusterData {

				n := map[string]interface{}{
					"id":          cluster.ID,
					"name":        cluster.Name,
					"description": cluster.Description,
				}
				clusters = append(clusters, n)
			}
			err = d.Set("clusters", clusters)
			if err != nil {
				return diag.FromErr(err)
			}
			d.SetId(time.Now().UTC().Format(time.RFC3339Nano))
		}
	} else {
		return diag.Errorf("Error retrieving clusters")
	}
	return diags
}

func dataSourceClusters() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceClustersRead,
		Schema: map[string]*schema.Schema{
			"filter_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: `If specified, results will be filtered to name`,
			},
			"clusters": {
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

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

// MediaSources structure to store version specific data in
type MediaSources struct {
	ID          int    `json:"$key,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	FileSize    int    `json:"filesize,omitempty"`
}

// MediaSourcesEndpoint is the api endpoint representing this resource
const MediaSourcesEndpoint = "api/v4/files"

func dataSourceMediaSourcesRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	opts := Options{Fields: "$key,name,description,filesize"}
	opts.Filter = "owner eq null"
	if fn := d.Get("filter_name"); fn != nil && fn != "" {
		opts.Filter += fmt.Sprintf(" and name eq '%s'", fn.(string))
	}

	resp, err := c.Get(MediaSourcesEndpoint, &opts)
	if err != nil {
		return diag.FromErr(err)
	}
	if resp != nil {
		if resp.StatusCode == 200 {
			var mediaSourcesData []MediaSources
			body, readerr := ioutil.ReadAll(resp.Body)
			if readerr != nil {
				return diag.FromErr(readerr)
			}

			decodeerr := json.Unmarshal(body, &mediaSourcesData)
			if decodeerr != nil {
				return diag.FromErr(decodeerr)
			}
			var mediaSources []map[string]interface{}

			for _, mediaSource := range mediaSourcesData {

				n := map[string]interface{}{
					"id":          mediaSource.ID,
					"name":        mediaSource.Name,
					"description": mediaSource.Description,
					"filesize":    mediaSource.FileSize,
				}
				mediaSources = append(mediaSources, n)
			}
			err = d.Set("mediasources", mediaSources)
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

func dataSourceMediaSources() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceMediaSourcesRead,
		Schema: map[string]*schema.Schema{
			"filter_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: `If specified, results will be filtered to name`,
			},
			"mediasources": {
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
						"filesize": {
							Type:     schema.TypeInt,
							Computed: true,
						},
					},
				},
			},
		},
	}
}

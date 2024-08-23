package vergeio

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
//	"log"
	"time"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Virtual Machines structure to store version specific data in
type VMs struct {
	ID          int    `json:"machine,omitempty"`
	Name        string `json:"name,omitempty"`
	Key         int    `json:"$key,omitempty"`
	IsSnapshot  bool   `json:"is_snapshot,omitempty"`
}

// VMsEndpoint is the api endpoint representing this resource
const VMsEndpoint = "api/v4/vms"

func dataSourceVMsRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	opts := Options{Fields: "machine,name,$key,is_snapshot"}
	
	// Build filter
	var filters []string
	if fn := d.Get("filter_name"); fn != nil && fn != "" {
		filters = append(filters, fmt.Sprintf("name eq '%s'", fn.(string)))
	}
	if len(filters) > 0 {
		opts.Filter = strings.Join(filters, " and ")
	}

	resp, err := c.Get(VMsEndpoint, &opts)
	if err != nil {
		return diag.FromErr(err)
	}
	if resp != nil {
		if resp.StatusCode == 200 {
			var vmsData []VMs
			body, readerr := ioutil.ReadAll(resp.Body)
			if readerr != nil {
				return diag.FromErr(readerr)
			}

			decodeerr := json.Unmarshal(body, &vmsData)
			if decodeerr != nil {
				return diag.FromErr(decodeerr)
			}
			var vms []map[string]interface{}

			isSnapshotFilter, isSnapshotFilterSet := d.GetOkExists("is_snapshot")

			for _, vm := range vmsData {
				if isSnapshotFilterSet {
					if vm.IsSnapshot != isSnapshotFilter.(bool) {
						continue
					}
				}

				n := map[string]interface{}{
					"id":          vm.ID,
					"name":        vm.Name,
					"key":         vm.Key,
					"is_snapshot": vm.IsSnapshot,
				}
				vms = append(vms, n)
			}
			err = d.Set("vms", vms)
			if err != nil {
				return diag.FromErr(err)
			}
			d.SetId(time.Now().UTC().Format(time.RFC3339Nano))
		}
	} else {
		return diag.Errorf("Error retrieving virtual machines")
	}
	return diags
}

func dataSourceVMs() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceVMsRead,
		Schema: map[string]*schema.Schema{
			"filter_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: `If specified, results will be filtered to name`,
			},
			"is_snapshot": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: `If specified, results will be filtered to VMs that are snapshots or not`,
			},
			"vms": {
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
						"key": {
							Type:     schema.TypeInt,
							Computed: true,
						},
						"is_snapshot": {
							Type:     schema.TypeBool,
							Computed: true,
						},
					},
				},
			},
		},
	}
}

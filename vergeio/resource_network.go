package vergeio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// NetworkEndPoint is the api endpoint representing this resource
const NetworkEndPoint = "api/v4/vnets"

// Network is the data structure for virtual machines in vergeos
type Network struct {
	Name            string `json:"name,omitempty"`
	Enabled         bool   `json:"enabled"`
	Default_Gateway int    `json:"vnet_default_gateway,omitempty"`
	IPaddress       string `json:"ipaddress,omitempty"`
	DHCP            bool   `json:"dhcp_enabled"`
	Dynamic_DHCP    bool   `json:"dhcp_dynamic"`
	DHCP_Sequential bool   `json:"dhcp_sequential"`
	DynamicIP_Start string `json:"dhcp_start,omitempty"`
	DynamicIP_Stop  string `json:"dhcp_stop,omitempty"`
	On_Power_Loss   string `json:"on_power_loss,omitempty"`
}

func newNetworkFromResource(d *schema.ResourceData) *Network {
	network := &Network{}
	if d.HasChange("name") {
		network.Name = d.Get("name").(string)
	}
	if d.HasChange("enabled") {
		network.Enabled = d.Get("enabled").(bool)
	}
	if d.HasChange("vnet_default_gateway") {
		network.Default_Gateway = d.Get("vnet_default_gateway").(int)
	}
	if d.HasChange("ipaddress") {
		network.IPaddress = d.Get("ipaddress").(string)
	}
	if d.HasChange("dhcp_enabled") {
		network.DHCP = d.Get("dhcp_enabled").(bool)
	}
	if d.HasChange("dhcp_dynamic") {
		network.Dynamic_DHCP = d.Get("dhcp_dynamic").(bool)
	}
	if d.HasChange("dhcp_sequential") {
		network.DHCP_Sequential = d.Get("dhcp_sequential").(bool)
	}
	if d.HasChange("dhcp_start") {
		network.DynamicIP_Start = d.Get("dhcp_start").(string)
	}
	if d.HasChange("dhcp_stop") {
		network.DynamicIP_Stop = d.Get("dhcp_stop").(string)
	}
	if d.HasChange("on_power_loss") {
		network.On_Power_Loss = d.Get("on_power_loss").(string)
	}
	return network
}

func resourceNetwork() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNetworkCreate,
		ReadContext:   resourceNetworkRead,
		UpdateContext: resourceNetworkUpdate,
		DeleteContext: resourceNetworkDelete,
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"enabled": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"vnet_default_gateway": {
				Type:     schema.TypeInt,
				Optional: true,
			},
			"ipaddress": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"dhcp_enabled": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"dynamic_dhcp": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"dhcp_sequential": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"dhcp_start": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"dhcp_stop": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"on_power_loss": {
				Type: schema.TypeString,
				ValidateFunc: validation.StringInSlice([]string{
					"power_on",
					"leave_off",
					"last_state",
				}, false),
				Optional: true,
			},
		},
	}
}

func resourceNetworkUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {

	client := m.(*Client)
	resource := newNetworkFromResource(d)
	bytedata, err := json.Marshal(resource)
	log.Printf("[DEBUG] resource data %s", string(bytedata))
	if err != nil {
		return diag.FromErr(err)
	}
	req, err := client.Put(fmt.Sprintf("%s/%s",
		NetworkEndPoint,
		d.Id(),
	), bytes.NewBuffer(bytedata))

	if err != nil {
		return diag.FromErr(err)
	}

	if req.StatusCode != 200 {
		return diag.Errorf(fmt.Sprintf("Error updating resource: %d", req.StatusCode))
	}
	return resourceNetworkRead(ctx, d, m)
}

func resourceNetworkCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)
	resource := newNetworkFromResource(d)
	bytedata, err := json.Marshal(&resource)
	if err != nil {
		return diag.FromErr(err)
	}

	request, err := c.Post(NetworkEndPoint, bytes.NewBuffer(bytedata))
	if err != nil {
		return diag.FromErr(err)
	}
	body, readerr := ioutil.ReadAll(request.Body)
	if readerr != nil {
		return diag.FromErr(readerr)
	}

	var resp VergeResponse
	decodeerr := json.Unmarshal(body, &resp)
	if decodeerr != nil {
		return diag.FromErr(decodeerr)
	}
	if resp.Error != "" {
		return diag.Errorf(resp.Error)
	}
	d.SetId(string(resp.Key))
	return resourceNetworkRead(ctx, d, m)
}

func resourceNetworkRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)

	var diags diag.Diagnostics

	request, err := c.Get(fmt.Sprintf("%s/%s",
		NetworkEndPoint,
		url.PathEscape(d.Id()),
	), nil)
	if request != nil && request.StatusCode == 404 {
		log.Printf("ID Not Found: %s", url.PathEscape(d.Id()))
		d.SetId("")
		return diags
	}
	if err != nil {
		return diag.FromErr(err)
	}

	log.Printf("ID: %s", url.PathEscape(d.Id()))
	var network Network
	if request != nil {
		if request.StatusCode == 200 {

			body, readerr := ioutil.ReadAll(request.Body)
			if readerr != nil {
				return diag.FromErr(readerr)
			}

			decodeerr := json.Unmarshal(body, &network)
			if decodeerr != nil {
				return diag.FromErr(decodeerr)
			}
			log.Printf("[DEBUG] params %#v", network)

		}
	} else {
		return diag.Errorf("Error retrieving network data")
	}

	d.Set("name", network.Name)
	d.Set("enabled", network.Enabled)
	d.Set("vnet_default_gateway", network.Default_Gateway)
	d.Set("ipaddress", network.IPaddress)
	d.Set("dhcp_enabled", network.DHCP)
	d.Set("dhcp_dynamic", network.Dynamic_DHCP)
	d.Set("dhcp_sequential", network.DHCP_Sequential)
	d.Set("dhcp_start", network.DynamicIP_Start)
	d.Set("dhcp_stop", network.DynamicIP_Stop)
	d.Set("on_power_loss", network.On_Power_Loss)
	return diags
}

func resourceNetworkDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	client := m.(*Client)
	_, err := client.Delete(fmt.Sprintf("%s/%s",
		NetworkEndPoint,
		d.Id(),
	))
	if err != nil {
		return diag.FromErr(err)
	}
	return diags
}

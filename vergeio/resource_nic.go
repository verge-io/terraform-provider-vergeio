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

// NICEndpoint is the api endpoint representing this resource
const NICEndpoint = "api/v4/machine_nics"

// NIC is the data structure for virtual machines in vergeos
type NIC struct {
	Machine     int    `json:"machine,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Interface   string `json:"interface,omitempty"`
	Driver      string `json:"driver,omitempty"`
	Model       string `json:"model,omitempty"`
	Vendor      string `json:"vendor,omitempty"`
	Port        int    `json:"port,omitempty"`
	Enabled     bool   `json:"enabled"`
	VNET        int    `json:"vnet,omitempty"`
	MAC         string `json:"macaddress,omitempty"`
	Asset       string `json:"asset,omitempty"`
}

func newNICFromResource(d *schema.ResourceData) *NIC {
	nic := &NIC{}
	if d.HasChange("machine") {
		nic.Machine = d.Get("machine").(int)
	}
	if d.HasChange("name") {
		nic.Name = d.Get("name").(string)
	}
	if d.HasChange("description") {
		nic.Description = d.Get("description").(string)
	}
	if d.HasChange("interface") {
		nic.Interface = d.Get("interface").(string)
	}
	if d.HasChange("driver") {
		nic.Driver = d.Get("driver").(string)
	}
	if d.HasChange("model") {
		nic.Model = d.Get("disksize").(string)
	}
	if d.HasChange("vendor") {
		nic.Vendor = d.Get("vendor").(string)
	}
	if d.HasChange("port") {
		nic.Port = d.Get("readonly").(int)
	}
	if d.HasChange("enabled") {
		nic.Enabled = d.Get("enabled").(bool)
	}
	if d.HasChange("vnet") {
		nic.VNET = d.Get("vnet").(int)
	}
	if d.HasChange("macaddress") {
		nic.MAC = d.Get("macaddress").(string)
	}
	if d.HasChange("asset") {
		nic.Asset = d.Get("asset").(string)
	}
	return nic
}

func resourceNIC() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNICCreate,
		ReadContext:   resourceNICRead,
		UpdateContext: resourceNICUpdate,
		DeleteContext: resourceNICDelete,
		Schema: map[string]*schema.Schema{
			"machine": {
				Type:     schema.TypeInt,
				Required: true,
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"interface": {
				Type:     schema.TypeString,
				Optional: true,
				ValidateFunc: validation.StringInSlice([]string{
					"virtio",
					"e1000",
					"rtl8139",
					"pcnet",
					"direct",
				}, false),
				Computed: true,
			},
			"driver": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"model": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"vendor": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"port": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"enabled": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"vnet": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"macaddress": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"asset": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
	}
}

func resourceNICUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {

	client := m.(*Client)
	resource := newNICFromResource(d)
	bytedata, err := json.Marshal(resource)
	log.Printf("[DEBUG] resource data %s", string(bytedata))
	if err != nil {
		return diag.FromErr(err)
	}
	req, err := client.Put(fmt.Sprintf("%s/%s",
		NICEndpoint,
		d.Id(),
	), bytes.NewBuffer(bytedata))

	if err != nil {
		return diag.FromErr(err)
	}

	if req.StatusCode != 200 {
		return diag.Errorf(fmt.Sprintf("Error updating resource: %d", req.StatusCode))
	}
	return resourceNICRead(ctx, d, m)
}

func resourceNICCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)
	resource := newNICFromResource(d)
	bytedata, err := json.Marshal(&resource)
	if err != nil {
		return diag.FromErr(err)
	}

	request, err := c.Post(NICEndpoint, bytes.NewBuffer(bytedata))
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
	return resourceNICRead(ctx, d, m)
}

func resourceNICRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)

	var diags diag.Diagnostics

	request, err := c.Get(fmt.Sprintf("%s/%s",
		NICEndpoint,
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
	var nic NIC
	if request != nil {
		if request.StatusCode == 200 {

			body, readerr := ioutil.ReadAll(request.Body)
			if readerr != nil {
				return diag.FromErr(readerr)
			}

			decodeerr := json.Unmarshal(body, &nic)
			if decodeerr != nil {
				return diag.FromErr(decodeerr)
			}
			log.Printf("[DEBUG] params %#v", nic)

		}
	} else {
		return diag.Errorf("Error retrieving nic data")
	}

	d.Set("machine", nic.Machine)
	d.Set("name", nic.Name)
	d.Set("description", nic.Description)
	d.Set("interface", nic.Interface)
	d.Set("driver", nic.Driver)
	d.Set("model", nic.Model)
	d.Set("vendor", nic.Vendor)
	d.Set("port", nic.Port)
	d.Set("vnet", nic.VNET)
	d.Set("macaddress", nic.MAC)
	d.Set("asset", nic.Asset)
	d.Set("enabled", nic.Enabled)

	return diags
}

func resourceNICDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	client := m.(*Client)
	_, err := client.Delete(fmt.Sprintf("%s/%s",
		NICEndpoint,
		d.Id(),
	))
	if err != nil {
		return diag.FromErr(err)
	}
	return diags
}

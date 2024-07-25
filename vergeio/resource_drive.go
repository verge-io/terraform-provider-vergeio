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

// DriveEndpoint is the api endpoint representing this resource
const DriveEndpoint = "api/v4/machine_drives"

// Drive is the data structure for virtual machines in vergeos
type Drive struct {
	Machine             int    `json:"machine,omitempty"`
	Name                string `json:"name,omitempty"`
	Description         string `json:"description,omitempty"`
	Interface           string `json:"interface,omitempty"`
	Media               string `json:"media,omitempty"`
	MediaSource         int    `json:"media_source,omitempty"`
	DiskSize            int    `json:"disksize,omitempty"`
	PreferredTier       string `json:"preferred_tier,omitempty"`
	Enabled             bool   `json:"enabled`
	ReadOnly            bool   `json:"readonly"`
	Serial              string `json:"serial,omitempty"`
	Asset               string `json:"asset,omitempty"`
	PreserveDriveFormat bool   `json:"preserve_drive_format"`
}

func newDriveFromResource(d *schema.ResourceData) *Drive {
	drive := &Drive{}
	if d.HasChange("machine") {
		drive.Machine = d.Get("machine").(int)
	}
	if d.HasChange("name") {
		drive.Name = d.Get("name").(string)
	}
	if d.HasChange("description") {
		drive.Description = d.Get("description").(string)
	}
	if d.HasChange("interface") {
		drive.Interface = d.Get("interface").(string)
	}
	if d.HasChange("media") {
		drive.Media = d.Get("media").(string)
	}
	if d.HasChange("media_source") {
		drive.MediaSource = d.Get("media_source").(int)
	}
	if d.HasChange("disksize") {
		drive.DiskSize = d.Get("disksize").(int)
	}
	if d.HasChange("preferred_tier") {
		drive.PreferredTier = d.Get("preferred_tier").(string)
	}
	if d.HasChange("enabled") {
		drive.Enabled = d.Get("enabled").(bool)
	}
	if d.HasChange("readonly") {
		drive.ReadOnly = d.Get("readonly").(bool)
	}
	if d.HasChange("serial") {
		drive.Serial = d.Get("serial").(string)
	}
	if d.HasChange("asset") {
		drive.Asset = d.Get("asset").(string)
	}
	if d.HasChange("preserve_drive_format") {
		drive.PreserveDriveFormat = d.Get("preserve_drive_format").(bool)
	}
	return drive
}

func resourceDrive() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceDriveCreate,
		ReadContext:   resourceDriveRead,
		UpdateContext: resourceDriveUpdate,
		DeleteContext: resourceDriveDelete,
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
					"ide",
					"ahci",
					"lsi53c895a",
					"megasas",
					"megasas-gen2",
					"mptsas1068",
					"virtio-scsi",
					"virtio-scsi-dedicated",
				}, false),
				Computed: true,
			},
			"media": {
				Type:     schema.TypeString,
				ForceNew: true,
				Optional: true,
				ValidateFunc: validation.StringInSlice([]string{
					"cdrom",
					"disk",
					"efidisk",
					"import",
					"clone",
					"nonpersistent",
				}, false),
				Computed: true,
			},
			"media_source": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"disksize": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"preferred_tier": {
				Type: schema.TypeString,
				ValidateFunc: validation.StringInSlice([]string{
					"1",
					"2",
					"3",
					"4",
					"5",
				}, false),
				Optional: true,
				Computed: true,
			},
			"enabled": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"readonly": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"serial": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"asset": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"preserve_drive_format": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
		},
	}
}

func resourceDriveUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {

	client := m.(*Client)
	resource := newDriveFromResource(d)
	bytedata, err := json.Marshal(resource)
	log.Printf("[DEBUG] resource data %s", string(bytedata))
	if err != nil {
		return diag.FromErr(err)
	}
	req, err := client.Put(fmt.Sprintf("%s/%s",
		DriveEndpoint,
		d.Id(),
	), bytes.NewBuffer(bytedata))

	if err != nil {
		return diag.FromErr(err)
	}

	if req.StatusCode != 200 {
		return diag.Errorf(fmt.Sprintf("Error updating resource: %d", req.StatusCode))
	}
	return resourceDriveRead(ctx, d, m)
}

func resourceDriveCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)
	resource := newDriveFromResource(d)
	bytedata, err := json.Marshal(&resource)
	if err != nil {
		return diag.FromErr(err)
	}

	request, err := c.Post(DriveEndpoint, bytes.NewBuffer(bytedata))
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
	return resourceDriveRead(ctx, d, m)
}

func resourceDriveRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)

	var diags diag.Diagnostics

	request, err := c.Get(fmt.Sprintf("%s/%s",
		DriveEndpoint,
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
	var drive Drive
	if request != nil {
		if request.StatusCode == 200 {

			body, readerr := ioutil.ReadAll(request.Body)
			if readerr != nil {
				return diag.FromErr(readerr)
			}

			decodeerr := json.Unmarshal(body, &drive)
			if decodeerr != nil {
				return diag.FromErr(decodeerr)
			}
			log.Printf("[DEBUG] params %#v", drive)

		}
	} else {
		return diag.Errorf("Error retrieving drive data")
	}

	d.Set("machine", drive.Machine)
	d.Set("name", drive.Name)
	d.Set("description", drive.Description)
	d.Set("interface", drive.Interface)
	d.Set("media", drive.Media)
	d.Set("media_source", drive.MediaSource)
	d.Set("disksize", drive.DiskSize)
	d.Set("preferred_tier", drive.PreferredTier)
	d.Set("enabled", drive.Enabled)
	d.Set("readonly", drive.ReadOnly)
	d.Set("serial", drive.Serial)
	d.Set("asset", drive.Asset)
	d.Set("preserve_drive_format", drive.PreserveDriveFormat)

	return diags
}

func resourceDriveDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	client := m.(*Client)
	_, err := client.Delete(fmt.Sprintf("%s/%s",
		DriveEndpoint,
		d.Id(),
	))
	if err != nil {
		return diag.FromErr(err)
	}
	return diags
}

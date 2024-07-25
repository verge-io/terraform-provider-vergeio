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

// VMEndpoint is the api endpoint representing this resource
const VMEndpoint = "api/v4/vms"

// VM is the data structure for virtual machines in vergeos
type VM struct {
	Machine            int    `json:"machine,omitempty"`
	Name               string `json:"name,omitempty"`
	Cluster            int    `json:"cluster,omitempty"`
	Description        string `json:"description,omitempty"`
	Enabled            bool   `json:"enabled"`
	MachineType        string `json:"machine_type"`
	AllowHotplug       bool   `json:"allow_hotplug"`
	DisablePowercycle  bool   `json:"disable_powercycle"`
	CPUCores           int    `json:"cpu_cores,omitempty"`
	CPUType            string `json:"cpu_type,omitempty"`
	RAM                int    `json:"ram,omitempty"`
	Console            string `json:"console,omitempty"`
	Display            string `json:"display,omitempty"`
	Video              string `json:"video,omitempty"`
	Sound              string `json:"sound,omitempty"`
	OSFamily           string `json:"os_family,omitempty"`
	OSDescription      string `json:"os_description,omitempty"`
	RTCBase            string `json:"rtc_base,omitempty"`
	BootOrder          string `json:"boot_order,omitempty"`
	ConsolePassEnabled bool   `json:"console_pass_enabled"`
	ConsolePass        string `json:"console_pass,omitempty"`
	USBTablet          bool   `json:"usb_tablet"`
	UEFI               bool   `json:"uefi"`
	SecureBoot         bool   `json:"secure_boot"`
	SerialPort         bool   `json:"serial_port"`
	BootDelay          int    `json:"boot_delay,omitempty"`
	PreferredNode      int    `json:"preferred_node,omitempty"`
	SnapshotProfile    int    `json:"snapshot_profile,omitempty"`
	//CloudInitDataSource string `json:"os_description,omitempty"`
}

func newVMFromResource(d *schema.ResourceData) *VM {
	vm := &VM{
		Name:               d.Get("name").(string),
		Description:        d.Get("description").(string),
		Enabled:            d.Get("enabled").(bool),
		MachineType:        d.Get("machine_type").(string),
		AllowHotplug:       d.Get("allow_hotplug").(bool),
		DisablePowercycle:  d.Get("disable_powercycle").(bool),
		CPUCores:           d.Get("cpu_cores").(int),
		CPUType:            d.Get("cpu_type").(string),
		RAM:                d.Get("ram").(int),
		Console:            d.Get("console").(string),
		Display:            d.Get("display").(string),
		Video:              d.Get("video").(string),
		Sound:              d.Get("sound").(string),
		OSFamily:           d.Get("os_family").(string),
		OSDescription:      d.Get("os_description").(string),
		RTCBase:            d.Get("rtc_base").(string),
		BootOrder:          d.Get("boot_order").(string),
		ConsolePassEnabled: d.Get("console_pass_enabled").(bool),
		ConsolePass:        d.Get("console_pass").(string),
		USBTablet:          d.Get("usb_tablet").(bool),
		UEFI:               d.Get("uefi").(bool),
		SecureBoot:         d.Get("secure_boot").(bool),
		SerialPort:         d.Get("serial_port").(bool),
		BootDelay:          d.Get("boot_delay").(int),
		PreferredNode:      d.Get("preferred_node").(int),
		SnapshotProfile:    d.Get("snapshot_profile").(int),
		Cluster:            d.Get("cluster").(int),
	}
	return vm
}

func resourceVM() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceVMCreate,
		ReadContext:   resourceVMRead,
		UpdateContext: resourceVMUpdate,
		DeleteContext: resourceVMDelete,
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"machine": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"enabled": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"machine_type": {
				Type:     schema.TypeString,
				Optional: true,
				ValidateFunc: validation.StringInSlice([]string{
					"pc",
					"pc-i440fx-2.7",
					"pc-i440fx-2.8",
					"pc-i440fx-2.9",
					"pc-i440fx-2.10",
					"pc-i440fx-2.11",
					"pc-i440fx-2.12",
					"pc-i440fx-3.0",
					"pc-i440fx-3.1",
					"pc-i440fx-4.0",
					"pc-i440fx-4.1",
					"pc-i440fx-4.2",
					"pc-i440fx-5.0",
					"pc-i440fx-5.1",
					"pc-i440fx-5.2",
					"pc-i440fx-6.0",
					"pc-i440fx-6.1",
					"pc-i440fx-6.2",
					"pc-i440fx-7.0",
					"pc-i440fx-7.1",
					"pc-i440fx-7.2",
					"pc-i440fx-8.0",
					"pc-i440fx-8.1",
					"q35",
					"pc-q35-2.7",
					"pc-q35-2.8",
					"pc-q35-2.9",
					"pc-q35-2.10",
					"pc-q35-2.11",
					"pc-q35-2.12",
					"pc-q35-3.0",
					"pc-q35-3.1",
					"pc-q35-4.0",
					"pc-q35-4.1",
					"pc-q35-4.2",
					"pc-q35-5.0",
					"pc-q35-5.1",
					"pc-q35-5.2",
					"pc-q35-6.0",
					"pc-q35-6.1",
					"pc-q35-6.2",
					"pc-q35-7.0",
					"pc-q35-7.1",
					"pc-q35-7.2",
					"pc-q35-8.0",
					"pc-q35-8.1",
					"yottabyte",
				}, false),
				Computed: true,
			},
			"allow_hotplug": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"disable_powercycle": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"cpu_cores": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"cpu_type": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"ram": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"console": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"display": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"video": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"sound": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"os_family": {
				Type:     schema.TypeString,
				Optional: true,
				ValidateFunc: validation.StringInSlice([]string{
					"linux",
					"windows",
					"freebsd",
					"other",
				}, false),
				Computed: true,
			},
			"os_description": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"rtc_base": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"boot_order": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"console_pass_enabled": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"console_pass": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"usb_tablet": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"uefi": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"secure_boot": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"serial_port": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"boot_delay": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"preferred_node": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"snapshot_profile": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"cluster": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
		},
	}
}

func resourceVMUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*Client)
	rvm := newVMFromResource(d)
	bytedata, err := json.Marshal(rvm)

	if err != nil {
		return diag.FromErr(err)
	}
	req, err := client.Put(fmt.Sprintf("%s/%s",
		VMEndpoint,
		d.Id(),
	), bytes.NewBuffer(bytedata))

	if err != nil {
		return diag.FromErr(err)
	}

	if req.StatusCode != 200 {
		return diag.Errorf(fmt.Sprintf("Error updating resource: %d", req.StatusCode))
	}
	return resourceVMRead(ctx, d, m)
}

func resourceVMCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)
	newVM := newVMFromResource(d)
	bytedata, err := json.Marshal(&newVM)
	if err != nil {
		return diag.FromErr(err)
	}

	VMReq, err := c.Post(VMEndpoint, bytes.NewBuffer(bytedata))
	if err != nil {
		return diag.FromErr(err)
	}
	body, readerr := ioutil.ReadAll(VMReq.Body)
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
	return resourceVMRead(ctx, d, m)
}

func resourceVMRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)
	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	VMReq, err := c.Get(fmt.Sprintf("%s/%s",
		VMEndpoint,
		url.PathEscape(d.Id()),
	), nil)
	if VMReq != nil && VMReq.StatusCode == 404 {
		log.Printf("ID Not Found: %s", url.PathEscape(d.Id()))
		d.SetId("")
		return diags
	}
	if err != nil {
		return diag.FromErr(err)
	}

	log.Printf("ID: %s", url.PathEscape(d.Id()))
	var vm VM
	if VMReq != nil {
		if VMReq.StatusCode == 200 {

			body, readerr := ioutil.ReadAll(VMReq.Body)
			if readerr != nil {
				return diag.FromErr(readerr)
			}

			decodeerr := json.Unmarshal(body, &vm)
			if decodeerr != nil {
				return diag.FromErr(decodeerr)
			}
			log.Printf("[DEBUG] params %#v", vm)

		}
	} else {
		return diag.Errorf("Error retrieving VM data")
	}
	d.Set("machine", vm.Machine)
	d.Set("name", vm.Name)
	d.Set("description", vm.Description)
	d.Set("enabled", vm.Enabled)
	d.Set("machine_type", vm.MachineType)
	d.Set("allow_hotplug", vm.AllowHotplug)
	d.Set("disable_powercycle", vm.DisablePowercycle)
	d.Set("cpu_cores", vm.CPUCores)
	d.Set("cpu_type", vm.CPUType)
	d.Set("ram", vm.RAM)
	d.Set("console", vm.Console)
	d.Set("display", vm.Display)
	d.Set("video", vm.Video)
	d.Set("sound", vm.Sound)
	d.Set("os_family", vm.OSFamily)
	d.Set("os_description", vm.OSDescription)
	d.Set("rtc_base", vm.RTCBase)
	d.Set("boot_order", vm.BootOrder)
	d.Set("console_pass_enabled", vm.ConsolePassEnabled)
	d.Set("console_pass", vm.ConsolePass)
	d.Set("usb_tablet", vm.USBTablet)
	d.Set("uefi", vm.UEFI)
	d.Set("secure_boot", vm.SecureBoot)
	d.Set("serial_port", vm.SerialPort)
	d.Set("boot_delay", vm.BootDelay)
	d.Set("preferred_node", vm.PreferredNode)
	d.Set("snapshot_profile", vm.SnapshotProfile)
	d.Set("cluster", vm.Cluster)
	return diags
}

func resourceVMDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	client := m.(*Client)
	_, err := client.Delete(fmt.Sprintf("%s/%s",
		VMEndpoint,
		d.Id(),
	))
	if err != nil {
		return diag.FromErr(err)
	}
	return diags
}

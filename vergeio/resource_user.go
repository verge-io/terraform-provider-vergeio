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

// UserEndpoint is the api endpoint representing this resource
const UserEndpoint = "api/v4/users"

// User is the data structure for virtual machines in vergeos
type User struct {
	AuthSource     int    `json:"auth_source,omitempty"`
	Name           string `json:"name,omitempty"`
	RemoteName     string `json:"remote_name,omitempty"`
	Enabled        bool   `json:"enabled"`
	DisplayName    string `json:"displayname,omitempty"`
	Email          string `json:"email,omitempty"`
	Type           string `json:"type,omitempty"`
	Password       string `json:"password,omitempty"`
	ChangePassword bool   `json:"change_password"`
}

func newUserFromResource(d *schema.ResourceData) *User {
	user := &User{}
	if d.HasChange("auth_source") {
		user.AuthSource = d.Get("auth_source").(int)
	}
	if d.HasChange("name") {
		user.Name = d.Get("name").(string)
	}
	if d.HasChange("remote_name") {
		user.RemoteName = d.Get("remote_name").(string)
	}
	if d.HasChange("enabled") {
		user.Enabled = d.Get("enabled").(bool)
	}
	if d.HasChange("displayname") {
		user.DisplayName = d.Get("displayname").(string)
	}
	if d.HasChange("email") {
		user.Email = d.Get("email").(string)
	}
	if d.HasChange("type") {
		user.Type = d.Get("type").(string)
	}
	if d.HasChange("password") {
		user.Password = d.Get("password").(string)
	}
	if d.HasChange("change_password") {
		user.ChangePassword = d.Get("change_password").(bool)
	}
	return user
}

func resourceUser() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceUserCreate,
		ReadContext:   resourceUserRead,
		UpdateContext: resourceUserUpdate,
		DeleteContext: resourceUserDelete,
		Schema: map[string]*schema.Schema{
			"auth_source": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"remote_name": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"enabled": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"displayname": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"email": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"type": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				ValidateFunc: validation.StringInSlice([]string{
					"normal",
					"api",
					"vdi",
				}, false),
				Computed: true,
			},
			"password": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"change_password": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
		},
	}
}

func resourceUserUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {

	client := m.(*Client)
	resource := newUserFromResource(d)
	bytedata, err := json.Marshal(resource)
	log.Printf("[DEBUG] resource data %s", string(bytedata))
	if err != nil {
		return diag.FromErr(err)
	}
	req, err := client.Put(fmt.Sprintf("%s/%s",
		UserEndpoint,
		d.Id(),
	), bytes.NewBuffer(bytedata))

	if err != nil {
		return diag.FromErr(err)
	}

	if req.StatusCode != 200 {
		return diag.Errorf(fmt.Sprintf("Error updating resource: %d", req.StatusCode))
	}
	return resourceUserRead(ctx, d, m)
}

func resourceUserCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)
	resource := newUserFromResource(d)
	bytedata, err := json.Marshal(&resource)
	if err != nil {
		return diag.FromErr(err)
	}

	request, err := c.Post(UserEndpoint, bytes.NewBuffer(bytedata))
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
	return resourceUserRead(ctx, d, m)
}

func resourceUserRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)

	var diags diag.Diagnostics

	request, err := c.Get(fmt.Sprintf("%s/%s",
		UserEndpoint,
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
	var user User
	if request != nil {
		if request.StatusCode == 200 {

			body, readerr := ioutil.ReadAll(request.Body)
			if readerr != nil {
				return diag.FromErr(readerr)
			}

			decodeerr := json.Unmarshal(body, &user)
			if decodeerr != nil {
				return diag.FromErr(decodeerr)
			}
			log.Printf("[DEBUG] params %#v", user)

		}
	} else {
		return diag.Errorf("Error retrieving user data")
	}

	d.Set("auth_source", user.AuthSource)
	d.Set("name", user.Name)
	d.Set("remote_name", user.RemoteName)
	d.Set("enabled", user.Enabled)
	d.Set("displayname", user.DisplayName)
	d.Set("email", user.Email)
	d.Set("type", user.Type)
	d.Set("password", user.Password)
	d.Set("change_password", user.ChangePassword)

	return diags
}

func resourceUserDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	client := m.(*Client)
	_, err := client.Delete(fmt.Sprintf("%s/%s",
		UserEndpoint,
		d.Id(),
	))
	if err != nil {
		return diag.FromErr(err)
	}
	return diags
}

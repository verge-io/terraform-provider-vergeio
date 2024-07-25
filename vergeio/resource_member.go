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
)

// MemberEndpoint is the api endpoint representing this resource
const MemberEndpoint = "api/v4/members"

// Member is the data structure for virtual machines in vergeos
type Member struct {
	Group  int    `json:"parent_group,omitempty"`
	Member string `json:"member,omitempty"`
}

func newMemberFromResource(d *schema.ResourceData) *Member {
	member := &Member{}
	if d.HasChange("group") {
		member.Group = d.Get("group").(int)
	}
	if d.HasChange("member") {
		member.Member = d.Get("member").(string)
	}
	return member
}

func resourceMember() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceMemberCreate,
		ReadContext:   resourceMemberRead,
		UpdateContext: resourceMemberUpdate,
		DeleteContext: resourceMemberDelete,
		Schema: map[string]*schema.Schema{
			"group": {
				Type:     schema.TypeInt,
				Required: true,
			},
			"member": {
				Type:     schema.TypeString,
				Required: true,
			},
		},
	}
}

func resourceMemberUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {

	client := m.(*Client)
	resource := newMemberFromResource(d)
	bytedata, err := json.Marshal(resource)
	log.Printf("[DEBUG] resource data %s", string(bytedata))
	if err != nil {
		return diag.FromErr(err)
	}
	req, err := client.Put(fmt.Sprintf("%s/%s",
		MemberEndpoint,
		d.Id(),
	), bytes.NewBuffer(bytedata))

	if err != nil {
		return diag.FromErr(err)
	}

	if req.StatusCode != 200 {
		return diag.Errorf(fmt.Sprintf("Error updating resource: %d", req.StatusCode))
	}
	return resourceMemberRead(ctx, d, m)
}

func resourceMemberCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)
	resource := newMemberFromResource(d)
	bytedata, err := json.Marshal(&resource)
	if err != nil {
		return diag.FromErr(err)
	}

	request, err := c.Post(MemberEndpoint, bytes.NewBuffer(bytedata))
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
	return resourceMemberRead(ctx, d, m)
}

func resourceMemberRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)

	var diags diag.Diagnostics

	request, err := c.Get(fmt.Sprintf("%s/%s",
		MemberEndpoint,
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
	var member Member
	if request != nil {
		if request.StatusCode == 200 {

			body, readerr := ioutil.ReadAll(request.Body)
			if readerr != nil {
				return diag.FromErr(readerr)
			}

			decodeerr := json.Unmarshal(body, &member)
			if decodeerr != nil {
				return diag.FromErr(decodeerr)
			}
			log.Printf("[DEBUG] params %#v", member)

		}
	} else {
		return diag.Errorf("Error retrieving member data")
	}

	d.Set("group", member.Group)
	d.Set("member", member.Member)

	return diags
}

func resourceMemberDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	client := m.(*Client)
	_, err := client.Delete(fmt.Sprintf("%s/%s",
		MemberEndpoint,
		d.Id(),
	))
	if err != nil {
		return diag.FromErr(err)
	}
	return diags
}

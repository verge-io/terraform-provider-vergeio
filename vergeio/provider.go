package vergeio

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Provider -
func Provider() *schema.Provider {
	return &schema.Provider{
		ConfigureFunc: providerConfigure,
		Schema: map[string]*schema.Schema{
			"host": {
				Required:    true,
				Type:        schema.TypeString,
				DefaultFunc: schema.EnvDefaultFunc("VERGEIO_HOST", nil),
				//ValidateFunc: validation.StringMatch(regexp.MustCompile(`^https://`), "Host must begin with https://"),
			},
			"username": {
				Required:    true,
				Type:        schema.TypeString,
				DefaultFunc: schema.EnvDefaultFunc("VERGEIO_USERNAME", nil),
			},
			"password": {
				Type:        schema.TypeString,
				Required:    true,
				Sensitive:   true,
				DefaultFunc: schema.EnvDefaultFunc("VERGEIO_PASSWORD", nil),
			},
			"insecure": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Disable SSL certificate verification",
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"vergeio_vm":      resourceVM(),
			"vergeio_drive":   resourceDrive(),
			"vergeio_nic":     resourceNIC(),
			"vergeio_user":    resourceUser(),
			"vergeio_member":  resourceMember(),
			"vergeio_network": resourceNetwork(),
		},
		DataSourcesMap: map[string]*schema.Resource{
			"vergeio_version":      dataSourceVersion(),
			"vergeio_clusters":     dataSourceClusters(),
			"vergeio_mediasources": dataSourceMediaSources(),
			"vergeio_nodes":        dataSourceNodes(),
			"vergeio_networks":     dataSourceNetworks(),
			"vergeio_groups":       dataSourceGroups(),
		},
	}
}

func providerConfigure(d *schema.ResourceData) (interface{}, error) {
	client := Client{
		Username: d.Get("username").(string),
		Password: d.Get("password").(string),
		Host:     d.Get("host").(string),
		Insecure: d.Get("insecure").(bool),
	}
	return &client, nil
}

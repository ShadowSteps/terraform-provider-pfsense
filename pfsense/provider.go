package pfsense

import (
	"crypto/tls"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

type providerConfiguration struct {
	Client *resty.Client
	Mutex  *sync.Mutex
	Cond   *sync.Cond
}

// Provider - Terrafrom properties for proxmox
func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"pf_client_id": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("PF_CLIENT_ID", nil),
				ValidateFunc: validation.All(validation.StringIsNotEmpty, validation.StringIsNotWhiteSpace),
			},
			"pf_api_token": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("PF_API_TOKEN", nil),
				Sensitive:   true,
				ValidateFunc:  validation.All(validation.StringIsNotEmpty, validation.StringIsNotWhiteSpace),
			},
			"pf_api_url": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("PF_API_URL", nil),
				ValidateFunc: validation.IsURLWithHTTPS,
			},
			"pf_tls_insecure": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"pf_timeout": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  300,
				ValidateFunc: validation.IntAtLeast(0),
			},
		},

		ResourcesMap: map[string]*schema.Resource{
			"pfsense_dhcp_static_mapping": resourceDhcpStaticMapping(),
			"pfsense_nat_port_forward": resourceNatPortForward(),
			"pfsense_alias": resourceAlias(),
		},

		ConfigureFunc: providerConfigure,
	}
}

func providerConfigure(d *schema.ResourceData) (interface{}, error) {
	client, err := getClient(d.Get("pf_api_url").(string), d.Get("pf_client_id").(string), d.Get("pf_api_token").(string), d.Get("pf_tls_insecure").(bool), d.Get("pf_timeout").(int))
	if err != nil {
		return nil, err
	}
	var mut sync.Mutex
	return &providerConfiguration{
		Client: client,
		Mutex:  &mut,
		Cond:   sync.NewCond(&mut),
	}, nil
}

func getClient(apiUrl string, clientId string, apiKey string, tlsInsecure bool, timeout int) (*resty.Client, error) {
	tlsconf := &tls.Config{InsecureSkipVerify: true}
	if !tlsInsecure {
		tlsconf = nil
	}
	client := resty.New()
	client.SetTLSClientConfig(tlsconf)
	client.SetTimeout(time.Duration(timeout) * time.Second)
	client.SetHostURL(apiUrl)

	authRequest := map[string]interface{}{
		"client-id":    clientId,
		"client-token": apiKey,
	}

	resp, err := client.R().
		SetBody(authRequest).
		SetResult(&AuthTokenResponse{}).
		ForceContentType("application/json").
		Post(PFSenseApiUri.Auth)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("failed to authenticate! code: %d", resp.StatusCode())
	}

	var result = resp.Result().(*AuthTokenResponse)

	if len(result.Data.Token) <= 0 {
		return nil, fmt.Errorf("failed to get token! data %s, result: %s", resp, result)
	}

	client.SetAuthToken(result.Data.Token)

	return client, nil
}

type ApiBaseResponse struct {
	Status  string `json:"status"`
	Code    int	   `json:"code"`
	Message string `json:"message"`
	Return  int	   `json:"return"`
}

type AuthTokenResponse struct {
	ApiBaseResponse
	Data 	struct{
		Token	string `json:"token"`
	}  `json:"data"`
}

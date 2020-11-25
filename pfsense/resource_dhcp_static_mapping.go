package pfsense

import (
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"
	"regexp"
	"time"
)

func resourceDhcpStaticMapping() *schema.Resource {
	return &schema.Resource{
		Create: resourceDhcpStaticMappingCreate,
		Read:   resourceDhcpStaticMappingRead,
		Update: resourceDhcpStaticMappingUpdate,
		Delete: resourceDhcpStaticMappingDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"interface": {
				Type:     schema.TypeString,
				Required: true,
				ValidateFunc: validation.All(validation.StringIsNotEmpty, validation.StringIsNotWhiteSpace),
			},
			"mac": {
				Type:     schema.TypeString,
				Required: true,
				ValidateFunc: validation.IsMACAddress,
			},
			"ipaddr": {
				Type:     schema.TypeString,
				Required: true,
				ValidateFunc: validation.IsIPAddress,
			},
			"client_identifier": {
				Type:     schema.TypeString,
				Optional: true,
				ValidateFunc:  validation.All(validation.StringIsNotEmpty, validation.StringIsNotWhiteSpace),
			},
			"hostname": {
				Type:     schema.TypeString,
				Optional: true,
				ValidateFunc:  validation.All(validation.StringIsNotEmpty, validation.StringIsNotWhiteSpace),
			},
		},
	}
}

func resourceDhcpStaticMappingCreate(d *schema.ResourceData, meta interface{}) error {
	pconf := meta.(*providerConfiguration)

	client := pconf.Client
	lock := pconf.Mutex

	fetchRequest := map[string]string{
		"interface": d.Get("interface").(string),
		"mac": d.Get("mac").(string),
	}

	lock.Lock()
	data, err := fetchDHCPRow(client, fetchRequest)
	if err != nil {
		lock.Unlock()
		return err
	}

	if data != nil {
		lock.Unlock()
		return fmt.Errorf("mapping for this mac already exists! data: %s", data)
	}

	var request = map[string]interface{}{
		"interface": d.Get("interface"),
		"mac": d.Get("mac"),
		"ipaddr": d.Get("ipaddr"),
	}

	var cid = d.Get("client_identifier")
	if cid != nil {
		request["cid"] = cid
	}

	var hostname = d.Get("hostname")
	if hostname != nil {
		request["hostname"] = hostname
	}

	resp, err := client.R().
		SetBody(request).
		Post(PFSenseApiUri.DHCPStaticMapping)

	if err != nil {
		lock.Unlock()
		return err
	}

	if resp.StatusCode() != 200 {
		lock.Unlock()
		return fmt.Errorf("invalid response code on create: %d", resp.StatusCode())
	}

	time.Sleep(100 * time.Millisecond)

	data, err1 := fetchDHCPRow(client, fetchRequest)
	if err1 != nil {
		lock.Unlock()
		return err1
	}
	if data == nil {
		lock.Unlock()
		return fmt.Errorf("mapping for this mac do not exists! data: %s", data)
	}

	d.SetId(dhcpResourceId(request["interface"].(string), data.Mac))
	lock.Unlock()

	err = resourceDhcpStaticMappingRead(d, meta)
	return err
}

func resourceDhcpStaticMappingRead(d *schema.ResourceData, meta interface{}) error {
	pconf := meta.(*providerConfiguration)
	lock := pconf.Mutex
	client := pconf.Client

	lock.Lock()
	iface, mac, err := parseDhcpResourceId(d.Id())
	if err != nil {
		d.SetId("")
		lock.Unlock()
		return err
	}

	fetchRequest := map[string]string{
		"interface": iface,
		"mac": mac,
	}

	data, err1 := fetchDHCPRow(client, fetchRequest)
	if err1 != nil {
		lock.Unlock()
		return err1
	}
	if data == nil {
		lock.Unlock()
		return fmt.Errorf("mapping for this id do not exists! request: %s, data: %s", fetchRequest, data)
	}

	d.SetId(dhcpResourceId(iface, data.Mac))

	err2 := d.Set("interface", iface)
	if err2 != nil {
		return err2
	}
	err2 = d.Set("ipaddr", data.Ipaddr)
	if err2 != nil {
		return err2
	}
	err2 = d.Set("hostname", data.Hostname)
	if err2 != nil {
		return err2
	}
	err2 = d.Set("client_identifier", data.Cid)
	if err2 != nil {
		return err2
	}
	err2 = d.Set("mac", data.Mac)
	if err2 != nil {
		return err2
	}
	lock.Unlock()

	return nil
}

func resourceDhcpStaticMappingDelete(d *schema.ResourceData, meta interface{}) error {
	pconf := meta.(*providerConfiguration)
	client := pconf.Client
	lock := pconf.Mutex

	iface, mac, err := parseDhcpResourceId(d.Id())
	if err != nil {
		d.SetId("")
		return err
	}
	fetchRequest := map[string]string{
		"interface": iface,
		"mac": mac,
	}

	lock.Lock()
	data, err1 := fetchDHCPRow(client, fetchRequest)
	if err1 != nil {
		lock.Unlock()
		return err1
	}
	if data == nil {
		lock.Unlock()
		return fmt.Errorf("mapping for this id do not exists! request: %s, data: %s", fetchRequest, data)
	}

	var request = map[string]interface{}{
		"id": data.Id,
		"interface": iface,
	}

	resp, err := client.R().
		SetBody(request).
		Delete(PFSenseApiUri.DHCPStaticMapping)

	lock.Unlock()
	if err != nil {
		return err
	}

	if resp.StatusCode() != 200 {
		return fmt.Errorf("invalid response code on delete: %d", resp.StatusCode())
	}

	return nil
}


func resourceDhcpStaticMappingUpdate(d *schema.ResourceData, meta interface{}) error {
	pconf := meta.(*providerConfiguration)
	client := pconf.Client
	lock := pconf.Mutex
	iface, mac, err := parseDhcpResourceId(d.Id())
	if err != nil {
		d.SetId("")
		return err
	}

	fetchRequest := map[string]string{
		"interface": iface,
		"mac": mac,
	}

	lock.Lock()
	data, err1 := fetchDHCPRow(client, fetchRequest)
	if err1 != nil {
		lock.Unlock()
		return err1
	}
	if data == nil {
		lock.Unlock()
		return fmt.Errorf("mapping for this id do not exists! request: %s, data: %s", fetchRequest, data)
	}

	var request = map[string]interface{}{
		"id": data.Id,
		"interface": iface,
		"mac": d.Get("mac"),
		"ipaddr": d.Get("ipaddr"),
	}

	var cid = d.Get("client_identifier")
	if cid != nil {
		request["cid"] = cid
	}

	var hostname = d.Get("hostname")
	if hostname != nil {
		request["hostname"] = hostname
	}

	resp, err := client.R().
		SetBody(request).
		Put(PFSenseApiUri.DHCPStaticMapping)

	lock.Unlock()
	if err != nil {
		return err
	}

	if resp.StatusCode() != 200 {
		return fmt.Errorf("invalid response code: %d", resp.StatusCode())
	}

	return nil
}

func dhcpResourceId(iface string, mac string) string {
	return fmt.Sprintf("%s/%s", iface, mac)
}

var rxRsId = regexp.MustCompile("([^/]+)/([^/]+)")

func parseDhcpResourceId(resId string) (iface string, mac string, err error) {
	if !rxRsId.MatchString(resId) {
		return "", "", fmt.Errorf("invalid resource format: %s. must be itnreface/mac", resId)
	}
	idMatch := rxRsId.FindStringSubmatch(resId)
	iface = idMatch[1]
	mac = idMatch[2]
	return
}

func fetchDHCPRow(client *resty.Client, request map[string]string) (*ReadDHCPStaticMapping, error) {
	resp, err := client.R().
		SetQueryParams(request).
		SetResult(&ReadDHCPStaticMappingMapResponse{}).
		ForceContentType("application/json").
		Get(PFSenseApiUri.DHCPStaticMapping)

	if err != nil {
		resp, err = client.R().
			SetQueryParams(request).
			SetResult(&ReadDHCPStaticMappingArrayResponse{}).
			ForceContentType("application/json").
			Get(PFSenseApiUri.DHCPStaticMapping)

		if err != nil {
			return nil, err
		}
		return nil, nil
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("invalid response code on fetch: %d", resp.StatusCode())
	}

	var result = resp.Result().(*ReadDHCPStaticMappingMapResponse)

	if len(result.Data) > 1 {
		return nil, fmt.Errorf("failed to retrieve id of created mapping! more then one result found or none. request: %s, data: %s", request, result)
	}

	for k := range result.Data {
		return result.Data[k], nil
	}

	return nil, nil
}

type ReadDHCPStaticMappingArrayResponse struct {
	ApiBaseResponse
	Data []*ReadDHCPStaticMapping `json:"data"`
}

type ReadDHCPStaticMappingMapResponse struct {
	ApiBaseResponse
	Data map[string]*ReadDHCPStaticMapping `json:"data"`
}

type ReadDHCPStaticMapping struct {
	Id int `json:"id"`
	Mac string `json:"mac"`
	Cid string `json:"cid"`
	Ipaddr string `json:"ipaddr"`
	Hostname string `json:"hostname"`
}

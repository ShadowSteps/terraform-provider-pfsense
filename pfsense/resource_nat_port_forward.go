package pfsense

import (
	"crypto/sha256"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"
	"regexp"
	"strconv"
	"time"
)

func resourceNatPortForward() *schema.Resource {
	return &schema.Resource{
		Create: resourceNatPortForwardCreate,
		Read:   resourceNatPortForwardRead,
		Update: resourceNatPortForwardUpdate,
		Delete: resourceNatPortForwardDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"interface": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.All(validation.StringIsNotEmpty, validation.StringIsNotWhiteSpace),
			},
			"protocol": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringInSlice([]string{"tcp", "udp", "tcp/udp"}, false),
			},
			"src": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.All(validation.StringIsNotEmpty, validation.StringIsNotWhiteSpace),
			},
			"dst": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.All(validation.StringIsNotEmpty, validation.StringIsNotWhiteSpace),
			},
			"srcport": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.All(validation.StringIsNotEmpty, validation.StringIsNotWhiteSpace),
			},
			"dstport": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.All(validation.StringIsNotEmpty, validation.StringIsNotWhiteSpace),
			},
			"target": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validation.IsIPAddress,
			},
			"local_port": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validation.All(validation.StringIsNotEmpty, validation.StringIsNotWhiteSpace),
			},
		},
	}
}

func resourceNatPortForwardCreate(d *schema.ResourceData, meta interface{}) error {
	pconf := meta.(*providerConfiguration)
	lock := pconf.Mutex
	client := pconf.Client

	request := map[string]interface{}{
		"interface":  d.Get("interface").(string),
		"protocol":   d.Get("protocol").(string),
		"src":        d.Get("src").(string),
		"dst":        d.Get("dst").(string),
		"srcport":    d.Get("srcport").(string),
		"dstport":    d.Get("dstport").(string),
		"target":     d.Get("target").(string),
		"local-port": d.Get("local_port").(string),
		"top":		  true,
		"apply":	  true,
	}

	lock.Lock()
	resp, err := client.R().
		SetBody(request).
		SetResult(&CreateNatPortForwardResponse{}).
		ForceContentType("application/json").
		Post(PFSenseApiUri.NATPortForward)

	if err != nil {
		lock.Unlock()
		return err
	}

	if resp.StatusCode() != 200 {
		lock.Unlock()
		return fmt.Errorf("invalid response code on create: %d, data: %s, request: %s", resp.StatusCode(), resp, request)
	}

	time.Sleep(100 * time.Millisecond)

	result := resp.Result().(*CreateNatPortForwardResponse)

	data, err1 := fetchNATList(client)
	if err1 != nil {
		lock.Unlock()
		return err1
	}
	if data == nil {
		lock.Unlock()
		return fmt.Errorf("nat list is empty! data: %s", data)
	}

	var id int = -1
	for key := range data {
		if data[key].hashCode() == result.Data.hashCode() {
			id = key
		}
	}

	if id < 0 {
		lock.Unlock()
		return fmt.Errorf("failed to find created NAT rule")
	}

	d.SetId(natResourceId(request["interface"].(string), id))

	lock.Unlock()
	return resourceNatPortForwardRead(d, meta)
}

func resourceNatPortForwardRead(d *schema.ResourceData, meta interface{}) error {
	pconf := meta.(*providerConfiguration)
	client := pconf.Client
	lock := pconf.Mutex
	iface, id, err := parseNatResourceId(d.Id())
	if err != nil {
		d.SetId("")
		return err
	}

	lock.Lock()
	data, err1 := fetchNATList(client)
	if err1 != nil {
		lock.Unlock()
		return err1
	}
	if data == nil {
		lock.Unlock()
		return fmt.Errorf("nat list is empty! data: %s", data)
	}

	nat := data[id]

	d.SetId(natResourceId(iface, id))

	err2 := d.Set("interface", iface)
	if err2 != nil {
		lock.Unlock()
		return err2
	}
	err2 = d.Set("protocol", nat.Protocol)
	if err2 != nil {
		lock.Unlock()
		return err2
	}
	err2 = d.Set("local_port", nat.LocalPort)
	if err2 != nil {
		lock.Unlock()
		return err2
	}
	err2 = d.Set("target", nat.Target)
	if err2 != nil {
		lock.Unlock()
		return err2
	}
	err2 = d.Set("dst", nat.Destination.toAddressString())
	if err2 != nil {
		lock.Unlock()
		return err2
	}
	err2 = d.Set("src", nat.Source.toAddressString())
	if err2 != nil {
		lock.Unlock()
		return err2
	}
	err2 = d.Set("srcport", nat.Source.getPort())
	if err2 != nil {
		lock.Unlock()
		return err2
	}
	err2 = d.Set("dstport", nat.Destination.getPort())
	if err2 != nil {
		lock.Unlock()
		return err2
	}

	lock.Unlock()
	return nil
}

func resourceNatPortForwardDelete(d *schema.ResourceData, meta interface{}) error {
	pconf := meta.(*providerConfiguration)
	client := pconf.Client
	lock := pconf.Mutex

	_, id, err := parseNatResourceId(d.Id())
	if err != nil {
		d.SetId("")
		return err
	}

	var request = map[string]interface{}{
		"id": id,
		"apply": true,
	}
	lock.Lock()
	resp, err := client.R().
		SetBody(request).
		Delete(PFSenseApiUri.NATPortForward)

	lock.Unlock()
	if err != nil {
		return err
	}

	if resp.StatusCode() != 200 {
		return fmt.Errorf("invalid response code on delete: %d", resp.StatusCode())
	}

	return nil
}

func resourceNatPortForwardUpdate(d *schema.ResourceData, meta interface{}) error {
	pconf := meta.(*providerConfiguration)
	client := pconf.Client
	lock := pconf.Mutex

	_, id, err := parseNatResourceId(d.Id())
	if err != nil {
		d.SetId("")
		return err
	}

	var request = map[string]interface{}{
		"id": id,
	}

	lock.Lock()
	resp, err := client.R().
		SetBody(request).
		Delete(PFSenseApiUri.NATPortForward)
	if err != nil {
		lock.Unlock()
		return err
	}

	requestCreate := map[string]interface{}{
		"interface":  d.Get("interface").(string),
		"protocol":   d.Get("protocol").(string),
		"src":        d.Get("src").(string),
		"dst":        d.Get("dst").(string),
		"srcport":    d.Get("srcport").(string),
		"dstport":    d.Get("dstport").(string),
		"target":     d.Get("target").(string),
		"local-port": d.Get("local_port").(string),
		"top":		  true,
		"apply":	  true,
	}

	resp, err = client.R().
		SetBody(requestCreate).
		SetResult(&CreateNatPortForwardResponse{}).
		ForceContentType("application/json").
		Post(PFSenseApiUri.NATPortForward)
	lock.Unlock()
	if err != nil {
		return err
	}

	if resp.StatusCode() != 200 {
		return fmt.Errorf("invalid response code on create: %d, data: %s, request: %s", resp.StatusCode(), resp, request)
	}

	return nil
}

func natResourceId(iface string, id int) string {
	return fmt.Sprintf("%s/%s", iface, strconv.Itoa(id))
}

var natRsId = regexp.MustCompile("([^/]+)/(\\d+)")

func parseNatResourceId(resId string) (iface string, id int, err error) {
	if !natRsId.MatchString(resId) {
		return "", -1, fmt.Errorf("invalid resource format: %s. must be itnreface/mac", resId)
	}
	idMatch := rxRsId.FindStringSubmatch(resId)
	iface = idMatch[1]
	id, err = strconv.Atoi(idMatch[2])
	return
}

func fetchNATList(client *resty.Client) ([]*NatPortForward, error) {
	resp, err := client.R().
		SetResult(&ReadNatPortForwardArrayResponse{}).
		ForceContentType("application/json").
		Get(PFSenseApiUri.NATPortForward)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("invalid response code on fetch: %d", resp.StatusCode())
	}

	var result = resp.Result().(*ReadNatPortForwardArrayResponse)

	if len(result.Data) <= 0 {
		return nil, nil
	}
	return result.Data, nil
}

type ReadNatPortForwardArrayResponse struct {
	ApiBaseResponse
	Data []*NatPortForward `json:"data"`
}

type ReadNatPortForwardMapResponse struct {
	ApiBaseResponse
	Data map[string]*NatPortForward `json:"data"`
}

type CreateNatPortForwardResponse struct {
	ApiBaseResponse
	Data *NatPortForward `json:"data"`
}

type NatPortForward struct {
	Interface   string                  `json:"interface"`
	Protocol    string                  `json:"protocol"`
	Source      *NatSourceOrDestination `json:"source"`
	Destination *NatSourceOrDestination `json:"destination"`
	Target      string                  `json:"target"`
	LocalPort   string                  `json:"local-port"`
}

type NatSourceOrDestination struct {
	Address string `json:"address"`
	Any     string `json:"any"`
	Network string `json:"network"`
	Not     string `json:"not"`
	Port    string `json:"port"`
}

func (d NatSourceOrDestination) toAddressString() string {
	var result = ""
	if len(d.Address) > 0 {
		result += d.Address
	} else if len(d.Network) > 0 {
		result += d.Network
	} else {
		result += "any"
	}
	return result
}

func (d NatSourceOrDestination) hashCode() string {
	return fmt.Sprintf("%x\n", sha256.Sum256([]byte(
		d.Port+d.Any+d.Network+d.Address+d.Not,
	)))
}

func (d NatSourceOrDestination) getPort() string {
	var result = "any"
	if len(d.Port) > 0 {
		result = d.Port
	}
	return result
}

func (d NatPortForward) hashCode() string {
	return fmt.Sprintf("%x\n", sha256.Sum256([]byte(
		d.Target+d.LocalPort+d.Protocol+d.Interface+d.Source.hashCode()+d.Destination.hashCode(),
	)))
}

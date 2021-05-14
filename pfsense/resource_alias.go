package pfsense

import (
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"
	"regexp"
	"strings"
	"time"
)

var aliasNameRegex = regexp.MustCompile("^([A-Za-z0-9_]+)$")

func resourceAlias() *schema.Resource {
	return &schema.Resource{
		Create: resourceAliasCreate,
		Read:   resourceAliasRead,
		Update: resourceAliasUpdate,
		Delete: resourceAliasDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ValidateFunc: validation.All(
					validation.StringIsNotEmpty,
					validation.StringIsNotWhiteSpace,
					validation.StringMatch(aliasNameRegex, "")),
			},
			"type": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringInSlice([]string{"host", "network", "port"}, false),
			},
			"desc": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"value": {
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"value": {
							Type:         schema.TypeString,
							Required:     true,
							ValidateFunc: validation.All(validation.StringIsNotEmpty, validation.StringIsNotWhiteSpace),
						},
						"details": {
							Type:     schema.TypeString,
							Optional: true,
						},
					},
				},
			},
		},
	}
}

func resourceAliasCreate(d *schema.ResourceData, meta interface{}) error {
	pconf := meta.(*providerConfiguration)

	client := pconf.Client
	lock := pconf.Mutex

	name := d.Get("name").(string)

	lock.Lock()
	data, err := fetchAlias(client, name)
	if err != nil {
		lock.Unlock()
		return err
	}

	if data != nil {
		lock.Unlock()
		return fmt.Errorf("alias for this name already exists! data: %s", data)
	}

	var request = map[string]interface{}{
		"name": name,
		"type": d.Get("type"),
	}

	var description = d.Get("desc")
	if description != nil {
		request["descr"] = description
	}

	var values = d.Get("value").([]interface{})
	valueCount := len(values)
	if valueCount == 1 {
		request["address"] = values[0].(map[string]interface{})["value"]
		request["detail"] = values[0].(map[string]interface{})["details"]
	} else {
		request["address"] = make([]interface{}, valueCount)
		request["detail"] = make([]interface{}, valueCount)
		for i, value := range values {
			request["address"].([]interface{})[i] = value.(map[string]interface{})["value"]
			request["detail"].([]interface{})[i] = value.(map[string]interface{})["details"]
		}
	}

	resp, err := client.R().
		SetBody(request).
		Post(PFSenseApiUri.Alias)

	if err != nil {
		lock.Unlock()
		return err
	}

	if resp.StatusCode() != 200 {
		lock.Unlock()
		return fmt.Errorf("invalid response code on create: %d, response: %s", resp.StatusCode(), resp.Body())
	}

	time.Sleep(100 * time.Millisecond)

	data, err1 := fetchAlias(client, name)
	if err1 != nil {
		lock.Unlock()
		return err1
	}
	if data == nil {
		lock.Unlock()
		return fmt.Errorf("allias for this name do not exists! data: %s", data)
	}

	d.SetId(aliasResourceId(name))
	lock.Unlock()

	err = resourceAliasRead(d, meta)
	return err
}

func resourceAliasRead(d *schema.ResourceData, meta interface{}) error {
	pconf := meta.(*providerConfiguration)
	lock := pconf.Mutex
	client := pconf.Client

	lock.Lock()
	name, err := parseAliasResourceId(d.Id())
	if err != nil {
		d.SetId("")
		lock.Unlock()
		return err
	}

	data, err1 := fetchAlias(client, name)
	if err1 != nil {
		lock.Unlock()
		return err1
	}
	if data == nil {
		lock.Unlock()
		return fmt.Errorf("alias for this name do not exists! name: %s, data: %s", name, data)
	}

	d.SetId(aliasResourceId(name))

	err2 := d.Set("name", name)
	if err2 != nil {
		return err2
	}
	err2 = d.Set("type", data.Type)
	if err2 != nil {
		return err2
	}
	err2 = d.Set("descr", data.Description)

	values := make([]map[string]string, 0)
	switch typ := data.Values.(type) {
	case []string:
		for i, value := range data.Values.([]string) {
			configValue := map[string]string{
				"value":   value,
				"details": data.Details.([]string)[i],
			}
			values = append(values, configValue)
		}
	case string:
		valueArray := strings.Split(data.Values.(string), " ")
		detailsArray := strings.Split(data.Details.(string), "||")
		for i, value := range valueArray {
			configValue := map[string]string{
				"value":   value,
				"details": detailsArray[i],
			}
			values = append(values, configValue)
		}
	default:
		return fmt.Errorf("result from data value list is not from supported type: %s", typ)
	}

	err2 = d.Set("value", values)
	if err2 != nil {
		return err2
	}
	lock.Unlock()

	return nil
}

func resourceAliasDelete(d *schema.ResourceData, meta interface{}) error {
	pconf := meta.(*providerConfiguration)
	client := pconf.Client
	lock := pconf.Mutex

	name, err := parseAliasResourceId(d.Id())
	if err != nil {
		d.SetId("")
		return err
	}

	lock.Lock()
	data, err1 := fetchAlias(client, name)
	if err1 != nil {
		lock.Unlock()
		return err1
	}
	if data == nil {
		lock.Unlock()
		return fmt.Errorf("alias for this id do not exists! name: %s, data: %s", name, data)
	}

	var request = map[string]interface{}{
		"id": name,
	}

	resp, err := client.R().
		SetBody(request).
		Delete(PFSenseApiUri.Alias)

	lock.Unlock()
	if err != nil {
		return err
	}

	if resp.StatusCode() != 200 {
		return fmt.Errorf("invalid response code on delete: %d, response %s, request: %s", resp.StatusCode(), resp.Body(), request)
	}

	return nil
}

func resourceAliasUpdate(d *schema.ResourceData, meta interface{}) error {
	pconf := meta.(*providerConfiguration)
	client := pconf.Client
	lock := pconf.Mutex
	name, err := parseAliasResourceId(d.Id())
	if err != nil {
		d.SetId("")
		return err
	}

	lock.Lock()
	data, err1 := fetchAlias(client, name)
	if err1 != nil {
		lock.Unlock()
		return err1
	}
	if data == nil {
		lock.Unlock()
		return fmt.Errorf("alias for this id do not exists! name: %s, data: %s", name, data)
	}

	var request = map[string]interface{}{
		"id":   name,
		"name": d.Get("name"),
		"type": d.Get("type"),
	}

	var description = d.Get("desc")
	if description != nil {
		request["descr"] = description
	}

	var values = d.Get("value").([]interface{})
	valueCount := len(values)
	if valueCount == 1 {
		request["address"] = values[0].(map[string]interface{})["value"]
		request["detail"] = values[0].(map[string]interface{})["details"]
	} else {
		request["address"] = make([]interface{}, valueCount)
		request["detail"] = make([]interface{}, valueCount)
		for i, value := range values {
			request["address"].([]interface{})[i] = value.(map[string]interface{})["value"]
			request["detail"].([]interface{})[i] = value.(map[string]interface{})["details"]
		}
	}

	resp, err := client.R().
		SetBody(request).
		Put(PFSenseApiUri.Alias)

	lock.Unlock()
	if err != nil {
		return err
	}

	if resp.StatusCode() != 200 {
		return fmt.Errorf("invalid response code: %d, response: %s, request: %s", resp.StatusCode(), resp.Body(), request)
	}

	return nil
}

func aliasResourceId(name string) string {
	return fmt.Sprintf("%s", name)
}

func parseAliasResourceId(resId string) (name string, err error) {
	if !aliasNameRegex.MatchString(resId) {
		return "", fmt.Errorf("invalid resource format: %s. must be %s", resId, aliasNameRegex.String())
	}
	name = resId
	return
}

func fetchAlias(client *resty.Client, aliasName string) (*ReadAlias, error) {
	resp, err := client.R().
		SetQueryParams(map[string]string{
			"name": aliasName,
		}).
		SetResult(&ReadAliasMapResponse{}).
		ForceContentType("application/json").
		Get(PFSenseApiUri.Alias)

	if err != nil {
		resp, err = client.R().
			SetQueryParams(map[string]string{
				"name": aliasName,
			}).
			SetResult(&ReadAliasArrayResponse{}).
			ForceContentType("application/json").
			Get(PFSenseApiUri.Alias)

		if err != nil {
			return nil, err
		}
		return nil, nil
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("invalid response code on fetch: %d", resp.StatusCode())
	}

	var result = resp.Result()

	for k := range result.(*ReadAliasMapResponse).Data {
		return result.(*ReadAliasMapResponse).Data[k], nil
	}

	return nil, nil
}

type ReadAliasArrayResponse struct {
	ApiBaseResponse
	Data []*ReadAlias `json:"data"`
}

type ReadAliasMapResponse struct {
	ApiBaseResponse
	Data map[string]*ReadAlias `json:"data"`
}

type ReadAlias struct {
	Id          int         `json:"id"`
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Description string      `json:"descr"`
	Values      interface{} `json:"address"`
	Details     interface{} `json:"detail"`
}

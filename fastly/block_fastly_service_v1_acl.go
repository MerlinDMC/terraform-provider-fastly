package fastly

import (
	"fmt"
	"log"

	gofastly "github.com/fastly/go-fastly/v2/fastly"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

type ACLServiceAttributeHandler struct {
	*DefaultServiceAttributeHandler
}

func NewServiceACL(sa ServiceMetadata) ServiceAttributeDefinition {
	return &ACLServiceAttributeHandler{
		&DefaultServiceAttributeHandler{
			key:             "acl",
			serviceMetadata: sa,
		},
	}
}

func (h *ACLServiceAttributeHandler) Process(d *schema.ResourceData, latestVersion int, conn *gofastly.Client) error {
	oldACLVal, newACLVal := d.GetChange(h.GetKey())
	if oldACLVal == nil {
		oldACLVal = new(schema.Set)
	}
	if newACLVal == nil {
		newACLVal = new(schema.Set)
	}

	oldACLSet := oldACLVal.(*schema.Set)
	newACLSet := newACLVal.(*schema.Set)

	remove := oldACLSet.Difference(newACLSet).List()
	add := newACLSet.Difference(oldACLSet).List()

	// Delete removed ACL configurations
	for _, vRaw := range remove {
		val := vRaw.(map[string]interface{})
		opts := gofastly.DeleteACLInput{
			ServiceID:      d.Id(),
			ServiceVersion: latestVersion,
			Name:           val["name"].(string),
		}

		log.Printf("[DEBUG] Fastly ACL removal opts: %#v", opts)
		err := conn.DeleteACL(&opts)

		if errRes, ok := err.(*gofastly.HTTPError); ok {
			if errRes.StatusCode != 404 {
				return err
			}
		} else if err != nil {
			return err
		}
	}

	// POST new ACL configurations
	for _, vRaw := range add {
		val := vRaw.(map[string]interface{})
		opts := gofastly.CreateACLInput{
			ServiceID:      d.Id(),
			ServiceVersion: latestVersion,
			Name:           val["name"].(string),
		}

		log.Printf("[DEBUG] Fastly ACL creation opts: %#v", opts)
		_, err := conn.CreateACL(&opts)
		if err != nil {
			return err
		}
	}
	return nil
}

func (h *ACLServiceAttributeHandler) Read(d *schema.ResourceData, s *gofastly.ServiceDetail, conn *gofastly.Client) error {

	log.Printf("[DEBUG] Refreshing ACLs for (%s)", d.Id())
	aclList, err := conn.ListACLs(&gofastly.ListACLsInput{
		ServiceID:      d.Id(),
		ServiceVersion: s.ActiveVersion.Number,
	})
	if err != nil {
		return fmt.Errorf("[ERR] Error looking up ACLs for (%s), version (%v): %s", d.Id(), s.ActiveVersion.Number, err)
	}

	al := flattenACLs(aclList)

	if err := d.Set(h.GetKey(), al); err != nil {
		log.Printf("[WARN] Error setting ACLs for (%s): %s", d.Id(), err)
	}

	return nil
}

func (h *ACLServiceAttributeHandler) Register(s *schema.Resource) error {
	s.Schema[h.GetKey()] = &schema.Schema{
		Type:     schema.TypeSet,
		Optional: true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				// Required fields
				"name": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Unique name to refer to this ACL",
				},
				// Optional fields
				"acl_id": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Generated acl id",
				},
			},
		},
	}
	return nil
}

func flattenACLs(aclList []*gofastly.ACL) []map[string]interface{} {
	var al []map[string]interface{}
	for _, acl := range aclList {
		// Convert VCLs to a map for saving to state.
		vclMap := map[string]interface{}{
			"acl_id": acl.ID,
			"name":   acl.Name,
		}

		// prune any empty values that come from the default string value in structs
		for k, v := range vclMap {
			if v == "" {
				delete(vclMap, k)
			}
		}

		al = append(al, vclMap)
	}

	return al
}

package fastly

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	gofastly "github.com/fastly/go-fastly/v12/fastly"
)

// H2RouterDomainServiceAttributeHandler provides implementation for H2 router domains (path-based routing).
// H2 router domains allow multiple services to share a domain using path-based routing.
// Example: cdn.example.com/service1, cdn.example.com/service2
type H2RouterDomainServiceAttributeHandler struct {
	*DefaultServiceAttributeHandler
}

// NewServiceH2RouterDomain returns a new resource for H2 router domains.
func NewServiceH2RouterDomain(sa ServiceMetadata) ServiceAttributeDefinition {
	return ToServiceAttributeDefinition(&H2RouterDomainServiceAttributeHandler{
		&DefaultServiceAttributeHandler{
			key:             "h2_router_domain",
			serviceMetadata: sa,
		},
	})
}

// Key returns the resource key.
func (h *H2RouterDomainServiceAttributeHandler) Key() string {
	return h.key
}

// GetSchema returns the resource schema for H2 router domains.
func (h *H2RouterDomainServiceAttributeHandler) GetSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeSet,
		Optional:    true,
		Description: "A set of H2 router domain names for path-based routing (e.g., cdn.example.com/service1). H2 router domains allow multiple services to share a domain using path components.",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"comment": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "An optional comment about the H2 router domain.",
				},
				"name": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "The H2 router domain with path (e.g., cdn.example.com/path). The domain must include both hostname and path components separated by a forward slash.",
					ValidateFunc: validateH2RouterDomainName,
				},
			},
		},
	}
}

// validateH2RouterDomainName validates that the H2 router domain has the correct format.
// H2 router domains must contain both a hostname and path component.
func validateH2RouterDomainName(v interface{}, k string) (warnings []string, errors []error) {
	value, ok := v.(string)
	if !ok {
		errors = append(errors, fmt.Errorf("expected type of %q to be string", k))
		return warnings, errors
	}

	if value == "" {
		errors = append(errors, fmt.Errorf("%q cannot be empty", k))
		return warnings, errors
	}

	// H2 router domains must contain a forward slash separating hostname from path
	if !strings.Contains(value, "/") {
		errors = append(errors, fmt.Errorf(
			"%q must be in format 'hostname/path' for H2 router domains (e.g., cdn.example.com/service1), got: %s",
			k, value))
		return warnings, errors
	}

	// Split into hostname and path
	parts := strings.SplitN(value, "/", 2)
	hostname := parts[0]
	path := parts[1]

	// Validate hostname part is not empty
	if hostname == "" {
		errors = append(errors, fmt.Errorf("%q hostname component cannot be empty", k))
		return warnings, errors
	}

	// Validate path part is not empty
	if path == "" {
		errors = append(errors, fmt.Errorf("%q path component cannot be empty (e.g., cdn.example.com/service1)", k))
		return warnings, errors
	}

	// Note: We intentionally do NOT validate RFC 1123 hostname format here.
	// The Fastly API will perform final validation. This allows for flexibility
	// in H2 router domain formats that may be supported by Fastly but not
	// strictly conforming to standard domain name rules.

	return warnings, errors
}

// Create creates the H2 router domain resource.
func (h *H2RouterDomainServiceAttributeHandler) Create(ctx context.Context, d *schema.ResourceData, resource map[string]any, serviceVersion int, conn *gofastly.Client) error {
	opts := gofastly.CreateDomainInput{
		ServiceID:      d.Id(),
		ServiceVersion: serviceVersion,
		Name:           gofastly.ToPointer(resource["name"].(string)),
	}

	if v, ok := resource["comment"]; ok {
		opts.Comment = gofastly.ToPointer(v.(string))
	}

	log.Printf("[DEBUG] Fastly H2 Router Domain Addition opts: %#v", opts)
	_, err := conn.CreateDomain(gofastly.NewContextForResourceID(ctx, d.Id()), &opts)
	if err != nil {
		return fmt.Errorf("error creating H2 router domain: %w", err)
	}
	return nil
}

// Read refreshes the H2 router domain resource.
func (h *H2RouterDomainServiceAttributeHandler) Read(ctx context.Context, d *schema.ResourceData, _ map[string]any, serviceVersion int, conn *gofastly.Client) error {
	localState := d.Get(h.GetKey()).(*schema.Set).List()

	if len(localState) > 0 || d.Get("imported").(bool) || d.Get("force_refresh").(bool) {
		log.Printf("[DEBUG] Refreshing H2 Router Domains for (%s)", d.Id())
		remoteState, err := conn.ListDomains(gofastly.NewContextForResourceID(ctx, d.Id()), &gofastly.ListDomainsInput{
			ServiceID:      d.Id(),
			ServiceVersion: serviceVersion,
		})
		if err != nil {
			return fmt.Errorf("error looking up H2 Router Domains for (%s), version (%v): %s", d.Id(), serviceVersion, err)
		}

		// Filter for H2 router domains (those containing "/")
		var h2RouterDomains []*gofastly.Domain
		for _, domain := range remoteState {
			if domain.Name != nil && strings.Contains(*domain.Name, "/") {
				h2RouterDomains = append(h2RouterDomains, domain)
			}
		}

		// Flatten H2 router domains
		dl := flattenH2RouterDomains(h2RouterDomains)

		if err := d.Set(h.GetKey(), dl); err != nil {
			log.Printf("[WARN] Error setting H2 Router Domains for (%s): %s", d.Id(), err)
		}
	}

	return nil
}

// Update updates the H2 router domain resource.
func (h *H2RouterDomainServiceAttributeHandler) Update(ctx context.Context, d *schema.ResourceData, resource, modified map[string]any, serviceVersion int, conn *gofastly.Client) error {
	opts := gofastly.UpdateDomainInput{
		ServiceID:      d.Id(),
		ServiceVersion: serviceVersion,
		Name:           resource["name"].(string),
	}

	if v, ok := modified["comment"]; ok {
		opts.Comment = gofastly.ToPointer(v.(string))
	}

	log.Printf("[DEBUG] Update H2 Router Domain Opts: %#v", opts)
	_, err := conn.UpdateDomain(gofastly.NewContextForResourceID(ctx, d.Id()), &opts)
	if err != nil {
		return fmt.Errorf("error updating H2 router domain: %w", err)
	}
	return nil
}

// Delete deletes the H2 router domain resource.
func (h *H2RouterDomainServiceAttributeHandler) Delete(ctx context.Context, d *schema.ResourceData, resource map[string]any, serviceVersion int, conn *gofastly.Client) error {
	opts := gofastly.DeleteDomainInput{
		ServiceID:      d.Id(),
		ServiceVersion: serviceVersion,
		Name:           resource["name"].(string),
	}

	log.Printf("[DEBUG] Fastly H2 Router Domain removal opts: %#v", opts)
	err := conn.DeleteDomain(gofastly.NewContextForResourceID(ctx, d.Id()), &opts)
	if errRes, ok := err.(*gofastly.HTTPError); ok {
		if errRes.StatusCode != 404 {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}

// flattenH2RouterDomains models data into format suitable for saving to Terraform state.
func flattenH2RouterDomains(remoteState []*gofastly.Domain) []map[string]any {
	result := make([]map[string]any, 0, len(remoteState))

	for _, resource := range remoteState {
		data := map[string]any{}
		if resource.Name != nil {
			data["name"] = *resource.Name
		}
		if resource.Comment != nil {
			data["comment"] = *resource.Comment
		}
		result = append(result, data)
	}

	return result
}

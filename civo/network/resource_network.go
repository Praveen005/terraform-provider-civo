package network

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"log"
	"time"

	"github.com/civo/civogo"
	"github.com/civo/terraform-provider-civo/internal/utils"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// ResourceNetwork function returns a schema.Resource that represents a Network.
// This can be used to create, read, update, and delete operations for a Network in the infrastructure.
func ResourceNetwork() *schema.Resource {
	return &schema.Resource{
		Description: "Provides a Civo network resource. This can be used to create, modify, and delete networks.",
		Schema: map[string]*schema.Schema{
			"label": {
				Type:         schema.TypeString,
				Required:     true,
				Description:  "Name for the network",
				ValidateFunc: utils.ValidateName,
			},
			"region": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Computed:    true,
				Description: "The region of the network",
			},
			"cidr_v4": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "The CIDR block for the network",
			},
			"nameservers_v4": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Computed:    true,
				Description: "List of nameservers for the network",
			},
			// Computed resource
			"name": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The name of the network",
			},
			"default": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "If the network is default, this will be `true`",
			},
			// VLAN Network
			"vlan_id": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "VLAN ID for the network",
			},
			"vlan_cidr_v4": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "CIDR for VLAN IPv4",
			},
			"vlan_gateway_ip_v4": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Gateway IP for VLAN IPv4",
			},
			"vlan_physical_interface": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Physical interface for VLAN",
			},
			"vlan_allocation_pool_v4_start": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Start of the IPv4 allocation pool for VLAN",
			},
			"vlan_allocation_pool_v4_end": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "End of the IPv4 allocation pool for VLAN",
			},
		},
		CreateContext: resourceNetworkCreate,
		ReadContext:   resourceNetworkRead,
		UpdateContext: resourceNetworkUpdate,
		DeleteContext: resourceNetworkDelete,
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(30 * time.Minute),
			Update: schema.DefaultTimeout(30 * time.Minute),
			Delete: schema.DefaultTimeout(30 * time.Minute),
		},
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		CustomizeDiff: customizeDiffNetwork,
	}
}

// function to create a new network
func resourceNetworkCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	apiClient := m.(*civogo.Client)

	// overwrite the region if is defined in the datasource
	if region, ok := d.GetOk("region"); ok {
		apiClient.Region = region.(string)
	}

	log.Printf("[INFO] creating the new network %s", d.Get("label").(string))
	vlanConfig := civogo.VLANConnectConfig{
		VlanID:                d.Get("vlan_id").(int),
		PhysicalInterface:     d.Get("vlan_physical_interface").(string),
		CIDRv4:                d.Get("vlan_cidr_v4").(string),
		GatewayIPv4:           d.Get("vlan_gateway_ip_v4").(string),
		AllocationPoolV4Start: d.Get("vlan_allocation_pool_v4_start").(string),
		AllocationPoolV4End:   d.Get("vlan_allocation_pool_v4_end").(string),
	}

	configs := civogo.NetworkConfig{
		Label:         d.Get("label").(string),
		CIDRv4:        d.Get("cidr_v4").(string),
		Region:        apiClient.Region,
		NameserversV4: expandStringList(d.Get("nameservers_v4")),
	}
	// Only add VLAN configuration if VLAN ID is set
	if vlanConfig.VlanID > 0 {
		configs.VLanConfig = &vlanConfig
	}

	log.Printf("[INFO] Attempting to create the network %s", d.Get("label").(string))
	network, err := apiClient.CreateNetwork(configs)
	if err != nil {
		customErr, parseErr := utils.ParseErrorResponse(err.Error())
		if parseErr == nil {
			err = customErr
		}
		return diag.Errorf("[ERR] failed to create network: %s", err)
	}

	d.SetId(network.ID)
	// Create a default firewall for the network
	log.Printf("[INFO] Creating default firewall for the network %s", d.Get("label").(string))
	err = createDefaultFirewall(apiClient, network.ID, network.Label)
	if err != nil {
		return diag.Errorf("[ERR] failed to create a new firewall for the network %s: %s", d.Get("label").(string), err)
	}
	return resourceNetworkRead(ctx, d, m)
}

// function to read a network
func resourceNetworkRead(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	apiClient := m.(*civogo.Client)

	// overwrite the region if is defined in the datasource
	if region, ok := d.GetOk("region"); ok {
		apiClient.Region = region.(string)
	}

	CurrentNetwork := civogo.Network{}

	log.Printf("[INFO] retriving the network %s", d.Id())
	resp, err := apiClient.ListNetworks()
	if err != nil {
		if resp == nil {
			d.SetId("")
			return nil
		}

		return diag.Errorf("[ERR] failed to list the network: %s", err)
	}

	for _, net := range resp {
		if net.ID == d.Id() {
			CurrentNetwork = net
		}
	}

	d.Set("name", CurrentNetwork.Name)
	d.Set("region", apiClient.Region)
	d.Set("label", CurrentNetwork.Label)
	d.Set("default", CurrentNetwork.Default)
	d.Set("cidr_v4", CurrentNetwork.CIDR)
	d.Set("nameservers_v4", CurrentNetwork.NameserversV4)

	return nil
}

// function to update the network
func resourceNetworkUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	apiClient := m.(*civogo.Client)

	// overwrite the region if is defined in the datasource
	if region, ok := d.GetOk("region"); ok {
		apiClient.Region = region.(string)
	}

	if d.HasChange("label") {
		log.Printf("[INFO] updating the network %s", d.Id())
		_, err := apiClient.RenameNetwork(d.Get("label").(string), d.Id())
		if err != nil {
			return diag.Errorf("[ERR] An error occurred while renaming the network %s", d.Id())
		}
	}

	networkConfig := civogo.NetworkConfig{
		Region:        apiClient.Region,
		NameserversV4: expandStringList(d.Get("nameservers_v4")),
	}

	if d.HasChange("nameservers_v4") {
		log.Printf("[INFO] updating the network nameservers %s", d.Id())
		_, err := apiClient.UpdateNetwork(d.Id(), networkConfig)
		if err != nil {
			return diag.Errorf("[ERR] An error occurred while updating the nameservers for the network %s: %s", d.Id(), err)
		}
	}
	return resourceNetworkRead(ctx, d, m)
}

// function to delete a network
func resourceNetworkDelete(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	apiClient := m.(*civogo.Client)

	// overwrite the region if is defined in the datasource
	if region, ok := d.GetOk("region"); ok {
		apiClient.Region = region.(string)
	}

	networkID := d.Id()
	log.Printf("[INFO] Deleting the network %s", networkID)

	deleteStateConf := &retry.StateChangeConf{
		Pending: []string{"deleting", "exists"},
		Target:  []string{"deleted"},
		Refresh: func() (interface{}, string, error) {
			// First, try to delete the network
			resp, err := apiClient.DeleteNetwork(networkID)
			if err != nil {
				return 0, "", err
			}
			// If delete was successful, start polling
			if resp.Result == "success" {
				// Check if the network still exists
				_, err := apiClient.GetNetwork(networkID)
				if err != nil {
					if errors.Is(err, civogo.DatabaseNetworkNotFoundError) {
						return resp, "deleted", nil
					}
					return nil, "", err
				}
				return resp, "deleting", nil
			}

			return resp, "exists", nil
		},
		Timeout:        60 * time.Minute,
		Delay:          5 * time.Second,
		MinTimeout:     3 * time.Second,
		NotFoundChecks: 10,
	}

	_, err := deleteStateConf.WaitForStateContext(context.Background())
	if err != nil {
		return diag.Errorf("error waiting for network (%s) to be deleted: %s", networkID, err)
	}

	return nil
}

func expandStringList(input interface{}) []string {
	var result []string

	if inputList, ok := input.([]interface{}); ok {
		for _, item := range inputList {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
	}
	return result
}

func customizeDiffNetwork(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
	if d.Id() != "" && d.HasChange("cidr_v4") {
		return fmt.Errorf("the 'cidr_v4' field is immutable")
	}
	return nil
}

// createDefaultFirewall function to create a default firewall
func createDefaultFirewall(apiClient *civogo.Client, networkID string, networkName string) error {

	firewallConfig := civogo.FirewallConfig{
		Name:      fmt.Sprintf("%s-default", networkName),
		NetworkID: networkID,
		Region:    apiClient.Region,
	}

	// Create the default firewall
	_, err := apiClient.NewFirewall(&firewallConfig)
	if err != nil {
		return err
	}
	return nil
}

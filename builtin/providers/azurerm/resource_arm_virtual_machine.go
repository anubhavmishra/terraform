package azurerm

import (
	"bytes"
	"fmt"
	"log"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/hashicorp/terraform/helper/hashcode"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceArmVirtualMachine() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmVirtualMachineCreate,
		Read:   resourceArmVirtualMachineRead,
		Update: resourceArmVirtualMachineUpdate,
		Delete: resourceArmVirtualMachineDelete,

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"location": &schema.Schema{
				Type:      schema.TypeString,
				Required:  true,
				ForceNew:  true,
				StateFunc: azureRMNormalizeLocation,
			},

			"resource_group_name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"plan": &schema.Schema{
				Type:     schema.TypeSet,
				Optional: true,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"publisher": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"product": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
				Set: resourceArmVirtualMachinePlanHash,
			},

			"availability_set_id": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},

			"vm_size": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"storage_image_reference": &schema.Schema{
				Type:     schema.TypeSet,
				Optional: true,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"publisher": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"offer": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"sku": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"version": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
				Set: resourceArmVirtualMachineStorageImageReferenceHash,
			},

			"storage_os_disk": &schema.Schema{
				Type:     schema.TypeSet,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"vhd_uri": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"caching": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},

						"create_option": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
				Set: resourceArmVirtualMachineStorageOsDiskHash,
			},

			"storage_data_disk": &schema.Schema{
				Type:     schema.TypeSet,
				Optional: true,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"vhd_uri": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"create_option": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"disk_size_gb": &schema.Schema{
							Type:     schema.TypeInt,
							Required: true,
						},

						"lun": &schema.Schema{
							Type:     schema.TypeInt,
							Required: true,
						},
					},
				},
				Set: resourceArmVirtualMachineStorageDataDiskHash,
			},

			"os_profile": &schema.Schema{
				Type:     schema.TypeSet,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"compute_name": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},

						"admin_username": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"admin_password": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"custom_data": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},

						"lun": &schema.Schema{
							Type:     schema.TypeInt,
							Required: true,
						},
					},
				},
				Set: resourceArmVirtualMachineStorageDataDiskHash,
			},

			"network_interface_ids": &schema.Schema{
				Type:     schema.TypeSet,
				Required: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Set:      schema.HashString,
			},
		},
	}
}

func resourceArmVirtualMachineCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient)
	vmClient := client.vmClient

	log.Printf("[INFO] preparing arguments for Azure ARM Virtual Machine creation.")

	name := d.Get("name").(string)
	location := d.Get("location").(string)
	resGroup := d.Get("resource_group_name").(string)
	network_profile := expandAzureRmVirtualMachineNetworkProfile(d)
	os_disk := expandAzureRmVirtualMachineOsDisk(d)
	vm_size := d.Get("vm_size").(string)

	storage_profile := compute.StorageProfile{
		OsDisk: &os_disk,
	}

	if _, ok := d.GetOk("storage_image_reference"); ok {
		image_ref, err := expandAzureRmVirtualMachineImageReference(d)
		if err != nil {
			return err
		}
		storage_profile.ImageReference = &image_ref
	}

	if _, ok := d.GetOk("storage_data_disk"); ok {
		data_disks, err := expandAzureRmVirtualMachineDataDisk(d)
		if err != nil {
			return err
		}
		storage_profile.DataDisks = &data_disks
	}

	properties := compute.VirtualMachineProperties{
		NetworkProfile: &network_profile,
		HardwareProfile: &compute.HardwareProfile{
			VMSize: compute.VirtualMachineSizeTypes(vm_size),
		},
		StorageProfile: &storage_profile,
	}

	if v, ok := d.GetOk("availability_set_id"); ok {
		availability_set := v.(string)
		availSet := compute.SubResource{
			ID: &availability_set,
		}

		properties.AvailabilitySet = &availSet
	}

	vm := compute.VirtualMachine{
		Name:       &name,
		Location:   &location,
		Properties: &properties,
	}

	if _, ok := d.GetOk("plan"); ok {

		plan, err := expandAzureRmVirtualMachinePlan(d)
		if err != nil {
			return err
		}

		vm.Plan = &plan
	}

	_, err := vmClient.CreateOrUpdate(resGroup, name, vm)
	if err != nil {
		return err
	}

	return resourceArmVirtualMachineRead(d, meta)
}

func resourceArmVirtualMachineRead(d *schema.ResourceData, meta interface{}) error {
	vmClient := meta.(*ArmClient).vmClient

	id, err := parseAzureResourceID(d.Id())
	if err != nil {
		return err
	}
	resGroup := id.ResourceGroup
	name := id.Path["virtualMachines"]

	resp, err := vmClient.Get(resGroup, name, "")
	if resp.StatusCode == http.StatusNotFound {
		d.SetId("")
		return nil
	}
	if err != nil {
		return fmt.Errorf("Error making Read request on Azure Virtual Machine %s: %s", name, err)
	}
	return nil
}

func resourceArmVirtualMachineUpdate(d *schema.ResourceData, meta interface{}) error {
	return resourceArmVirtualMachineRead(d, meta)
}

func resourceArmVirtualMachineDelete(d *schema.ResourceData, meta interface{}) error {
	vmClient := meta.(*ArmClient).vmClient

	id, err := parseAzureResourceID(d.Id())
	if err != nil {
		return err
	}
	resGroup := id.ResourceGroup
	name := id.Path["virtualMachines"]

	_, err = vmClient.Delete(resGroup, name)

	return err
}

func resourceArmVirtualMachinePlanHash(v interface{}) int {
	var buf bytes.Buffer
	m := v.(map[string]interface{})
	buf.WriteString(fmt.Sprintf("%s-", m["name"].(string)))
	buf.WriteString(fmt.Sprintf("%s-", m["publisher"].(string)))
	buf.WriteString(fmt.Sprintf("%s-", m["product"].(string)))

	return hashcode.String(buf.String())
}

func resourceArmVirtualMachineStorageImageReferenceHash(v interface{}) int {
	var buf bytes.Buffer
	m := v.(map[string]interface{})
	buf.WriteString(fmt.Sprintf("%s-", m["publisher"].(string)))
	buf.WriteString(fmt.Sprintf("%s-", m["offer"].(string)))
	buf.WriteString(fmt.Sprintf("%s-", m["sku"].(string)))
	buf.WriteString(fmt.Sprintf("%s-", m["version"].(string)))

	return hashcode.String(buf.String())
}

func resourceArmVirtualMachineStorageDataDiskHash(v interface{}) int {
	var buf bytes.Buffer
	m := v.(map[string]interface{})
	buf.WriteString(fmt.Sprintf("%s-", m["name"].(string)))
	buf.WriteString(fmt.Sprintf("%s-", m["vhd_uri"].(string)))
	buf.WriteString(fmt.Sprintf("%s-", m["create_option"].(string)))
	buf.WriteString(fmt.Sprintf("%d-", m["disk_size_gb"].(int)))
	buf.WriteString(fmt.Sprintf("%d-", m["lun"].(int)))

	return hashcode.String(buf.String())
}

func resourceArmVirtualMachineStorageOsDiskHash(v interface{}) int {
	var buf bytes.Buffer
	m := v.(map[string]interface{})
	buf.WriteString(fmt.Sprintf("%s-", m["name"].(string)))
	buf.WriteString(fmt.Sprintf("%s-", m["vhd_uri"].(string)))
	buf.WriteString(fmt.Sprintf("%s-", m["create_option"].(string)))

	return hashcode.String(buf.String())
}

func expandAzureRmVirtualMachinePlan(d *schema.ResourceData) (compute.Plan, error) {
	planconfig := d.Get("plan").([]interface{})

	if len(planconfig) == 1 {
		publisher := planconfig[0]["publisher"].(string)
		name := planconfig[0]["name"].(string)
		product := planconfig[0]["product"].(string)

		plan := compute.Plan{
			Publisher: &publisher,
			Name:      &name,
			Product:   &product,
		}

		return plan, nil

	} else {
		return nil, fmt.Errorf("Cannot specify more than one plan.")
	}

	return nil, nil
}

func expandAzureRmVirtualMachineDataDisk(d *schema.ResourceData) ([]compute.DataDisk, error) {
	disks := d.Get("storage_data_disk").([]interface{})
	data_disks := make([]compute.DataDisk, 0, len(disks))
	for _, disk_config := range disks {
		config := disk_config.(map[string]interface{})

		name := config["name"].(string)
		vhd := config["vhd_uri"].(string)
		createOption := config["create_option"].(string)
		lun := config["lun"].(int)
		disk_size := config["disk_size_gb"].(int)

		data_disk := compute.DataDisk{
			Name: &name,
			Vhd: &compute.VirtualHardDisk{
				URI: &vhd,
			},
			Lun:          &lun,
			DiskSizeGB:   &disk_size,
			CreateOption: compute.DiskCreateOptionTypes(createOption),
		}

		data_disks = append(data_disks, data_disk)
	}

	return data_disks, nil
}

func expandAzureRmVirtualMachineImageReference(d *schema.ResourceData) (compute.ImageReference, error) {
	ws := d.Get("storage_image_reference").([]interface{})

	if len(ws) == 1 {
		publisher := ws[0]["publisher"].(string)
		offer := ws[0]["offer"].(string)
		sku := ws[0]["sku"].(string)
		version := ws[0]["version"].(string)

		image_reference := compute.ImageReference{
			Publisher: &publisher,
			Offer:     &offer,
			Sku:       &sku,
			Version:   &version,
		}

		return image_reference, nil

	} else {
		return nil, fmt.Errorf("Cannot specify more than one storage_image_reference.")
	}

	return nil, nil
}

func expandAzureRmVirtualMachineNetworkProfile(d *schema.ResourceData) compute.NetworkProfile {
	nicIds := d.Get("network_interface_ids").(*schema.Set).List()
	network_interfaces := make([]compute.NetworkInterfaceReference, 0, len(nicIds))

	network_profile := compute.NetworkProfile{}

	for _, nic := range nicIds {
		id := nic.(string)
		network_interface := compute.NetworkInterfaceReference{
			ID: &id,
		}
		network_interfaces = append(network_interfaces, network_interface)
	}

	network_profile.NetworkInterfaces = &network_interfaces

	return network_profile
}

func expandAzureRmVirtualMachineOsDisk(d *schema.ResourceData) compute.OSDisk {
	disks := d.Get("storage_os_disk").(*schema.Set).List()
	if len(disks) > 1 {
		return fmt.Errorf("[ERROR] Only 1 OS Disk Can be specified for an Azure RM Virtual Machine")
	}

	compute.DataDisk{}

	name := disks[0]["name"].(string)
	vhd_uri := disks[0]["vhd_url"].(string)
	create_option := disks[0]["create_option"].(string)
	os_disk := compute.OSDisk{
		Name: &name,
		Vhd: &compute.VirtualHardDisk{
			URI: &vhd_uri,
		},
		CreateOption: compute.DiskCreateOptionTypes(create_option),
	}

	if v := disks[0]["cachine"].(string); v != "" {
		os_disk.Caching = compute.CachingTypes(v)
	}

	return os_disk
}

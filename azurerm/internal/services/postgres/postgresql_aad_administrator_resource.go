package sql

import (
	"fmt"
	"log"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/postgresql/mgmt/2017-12-01/postgresql"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"
	uuid "github.com/satori/go.uuid"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/azure"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/tf"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/clients"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/features"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/timeouts"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/utils"
)

func resourceArmPostgreSQLAdministrator() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmPostgreSQLAdministratorCreateUpdate,
		Read:   resourceArmPostgreSQLAdministratorRead,
		Update: resourceArmPostgreSQLAdministratorCreateUpdate,
		Delete: resourceArmPostgreSQLAdministratorDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(30 * time.Minute),
			Read:   schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(30 * time.Minute),
			Delete: schema.DefaultTimeout(30 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"server_name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"resource_group_name": azure.SchemaResourceGroupName(),

			"login": {
				Type:     schema.TypeString,
				Required: true,
			},

			"object_id": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.IsUUID,
			},

			"tenant_id": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.IsUUID,
			},
		},
	}
}

func resourceArmPostgreSQLAdministratorCreateUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).postgresql.ServerAdministratorsClient
	ctx, cancel := timeouts.ForCreateUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	serverName := d.Get("server_name").(string)
	resGroup := d.Get("resource_group_name").(string)
	login := d.Get("login").(string)
	objectId := uuid.FromStringOrNil(d.Get("object_id").(string))
	tenantId := uuid.FromStringOrNil(d.Get("tenant_id").(string))

	if features.ShouldResourcesBeImported() && d.IsNewResource() {
		existing, err := client.Get(ctx, resGroup, serverName)
		if err != nil {
			if !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("Error checking for presence of existing PostgreSQL AD Administrator (Resource Group %q, Server %q): %+v", resGroup, serverName, err)
			}
		}

		if existing.ID != nil && *existing.ID != "" {
			return tf.ImportAsExistsError("azurerm_postgresql_active_directory_administrator", *existing.ID)
		}
	}

	parameters := postgresql.ServerAdministratorResource{
		ServerAdministratorProperties: &postgresql.ServerAdministratorProperties{
			AdministratorType: utils.String("ActiveDirectory"),
			Login:             utils.String(login),
			Sid:               &objectId,
			TenantID:          &tenantId,
		},
	}

	future, err := client.CreateOrUpdate(ctx, resGroup, serverName, parameters)
	if err != nil {
		return fmt.Errorf("Error issuing create/update request for PostgreSQL AD Administrator (Resource Group %q, Server %q): %+v", resGroup, serverName, err)
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("Error waiting on create/update future for PostgreSQL AD Administrator (Resource Group %q, Server %q): %+v", resGroup, serverName, err)
	}

	resp, err := client.Get(ctx, resGroup, serverName)
	if err != nil {
		return fmt.Errorf("Error issuing get request for PostgreSQL AD Administrator (Resource Group %q, Server %q): %+v", resGroup, serverName, err)
	}

	d.SetId(*resp.ID)

	return nil
}

func resourceArmPostgreSQLAdministratorRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).postgresql.ServerAdministratorsClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := azure.ParseAzureResourceID(d.Id())
	if err != nil {
		return err
	}

	resourceGroup := id.ResourceGroup
	serverName := id.Path["servers"]

	resp, err := client.Get(ctx, resourceGroup, serverName)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			log.Printf("[INFO] Error reading PostgreSQL AD administrator %q - removing from state", d.Id())
			d.SetId("")
			return nil
		}

		return fmt.Errorf("Error reading PostgreSQL AD administrator: %+v", err)
	}

	d.Set("resource_group_name", resourceGroup)
	d.Set("server_name", serverName)
	d.Set("login", resp.Login)
	d.Set("object_id", resp.Sid.String())
	d.Set("tenant_id", resp.TenantID.String())

	return nil
}

func resourceArmPostgreSQLAdministratorDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).postgresql.ServerAdministratorsClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := azure.ParseAzureResourceID(d.Id())
	if err != nil {
		return err
	}

	resourceGroup := id.ResourceGroup
	serverName := id.Path["servers"]

	_, err = client.Delete(ctx, resourceGroup, serverName)
	if err != nil {
		return fmt.Errorf("Error deleting PostgreSQL AD Administrator: %+v", err)
	}

	return nil
}

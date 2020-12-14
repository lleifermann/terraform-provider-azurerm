package authorization

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/preview/authorization/mgmt/2018-09-01-preview/authorization"
	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/tf"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/clients"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/services/authorization/azuresdkhacks"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/services/authorization/parse"
	azSchema "github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/tf/schema"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/timeouts"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/utils"
)

func resourceArmRoleDefinition() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmRoleDefinitionCreate,
		Read:   resourceArmRoleDefinitionRead,
		Update: resourceArmRoleDefinitionUpdate,
		Delete: resourceArmRoleDefinitionDelete,

		Importer: azSchema.ValidateResourceIDPriorToImport(func(id string) error {
			_, err := parse.RoleDefinitionId(id)
			return err
		}),

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(30 * time.Minute),
			Read:   schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(30 * time.Minute),
			Delete: schema.DefaultTimeout(30 * time.Minute),
		},

		SchemaVersion: 1,

		StateUpgraders: []schema.StateUpgrader{
			{
				Type:    resourceArmRoleDefinitionV0().CoreConfigSchema().ImpliedType(),
				Upgrade: resourceArmRoleDefinitionStateUpgradeV0,
				Version: 0,
			},
		},

		Schema: map[string]*schema.Schema{
			"role_definition_id": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},

			"name": {
				Type:     schema.TypeString,
				Required: true,
			},

			"scope": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"description": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"permissions": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"actions": {
							Type:     schema.TypeList,
							Optional: true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
						"not_actions": {
							Type:     schema.TypeList,
							Optional: true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
						"data_actions": {
							Type:     schema.TypeSet,
							Optional: true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
							Set: schema.HashString,
						},
						"not_data_actions": {
							Type:     schema.TypeSet,
							Optional: true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
							Set: schema.HashString,
						},
					},
				},
			},

			"assignable_scopes": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},

			"role_definition_resource_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceArmRoleDefinitionCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Authorization.RoleDefinitionsClient
	ctx, cancel := timeouts.ForCreate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	roleDefinitionId := d.Get("role_definition_id").(string)
	if roleDefinitionId == "" {
		uuid, err := uuid.GenerateUUID()
		if err != nil {
			return fmt.Errorf("Error generating UUID for Role Assignment: %+v", err)
		}

		roleDefinitionId = uuid
	}

	name := d.Get("name").(string)
	scope := d.Get("scope").(string)
	description := d.Get("description").(string)
	roleType := "CustomRole"

	permissionsRaw := d.Get("permissions").([]interface{})
	permissions := expandRoleDefinitionPermissions(permissionsRaw)
	assignableScopes := expandRoleDefinitionAssignableScopes(d)

	if d.IsNewResource() {
		existing, err := client.Get(ctx, scope, roleDefinitionId)
		if err != nil {
			if !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("Error checking for presence of existing Role Definition ID for %q (Scope %q)", name, scope)
			}
		}

		if existing.ID != nil && *existing.ID != "" {
			importID := fmt.Sprintf("%s|%s", *existing.ID, scope)
			return tf.ImportAsExistsError("azurerm_role_definition", importID)
		}
	}

	properties := authorization.RoleDefinition{
		RoleDefinitionProperties: &authorization.RoleDefinitionProperties{
			RoleName:         utils.String(name),
			Description:      utils.String(description),
			RoleType:         utils.String(roleType),
			Permissions:      &permissions,
			AssignableScopes: &assignableScopes,
		},
	}

	if _, err := client.CreateOrUpdate(ctx, scope, roleDefinitionId, properties); err != nil {
		return err
	}

	read, err := client.Get(ctx, scope, roleDefinitionId)
	if err != nil {
		return err
	}
	if read.ID == nil || *read.ID == "" {
		return fmt.Errorf("Cannot read Role Definition ID for %q (Scope %q)", name, scope)
	}

	d.SetId(fmt.Sprintf("%s|%s", *read.ID, scope))
	return resourceArmRoleDefinitionRead(d, meta)
}

func resourceArmRoleDefinitionUpdate(d *schema.ResourceData, meta interface{}) error {
	sdkClient := meta.(*clients.Client).Authorization.RoleDefinitionsClient
	client := azuresdkhacks.NewRoleDefinitionsWorkaroundClient(sdkClient)
	ctx, cancel := timeouts.ForUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	roleDefinitionId, err := parse.RoleDefinitionId(d.Id())
	if err != nil {
		return err
	}

	name := d.Get("name").(string)
	description := d.Get("description").(string)
	roleType := "CustomRole"

	permissionsRaw := d.Get("permissions").([]interface{})
	permissions := expandRoleDefinitionPermissions(permissionsRaw)
	assignableScopes := expandRoleDefinitionAssignableScopes(d)

	properties := authorization.RoleDefinition{
		RoleDefinitionProperties: &authorization.RoleDefinitionProperties{
			RoleName:         utils.String(name),
			Description:      utils.String(description),
			RoleType:         utils.String(roleType),
			Permissions:      &permissions,
			AssignableScopes: &assignableScopes,
		},
	}

	resp, err := client.CreateOrUpdate(ctx, roleDefinitionId.Scope, roleDefinitionId.RoleID, properties)
	if err != nil {
		return fmt.Errorf("updating Role Definition %q (Scope %q): %+v", roleDefinitionId.RoleID, roleDefinitionId.Scope, err)
	}
	if resp.RoleDefinitionProperties == nil {
		return fmt.Errorf("updating Role Definition %q (Scope %q): `properties` was nil", roleDefinitionId.RoleID, roleDefinitionId.Scope)
	}
	updatedOn := resp.RoleDefinitionProperties.UpdatedOn
	if updatedOn == nil {
		return fmt.Errorf("updating Role Definition %q (Scope %q): `properties.UpdatedOn` was nil", roleDefinitionId.RoleID, roleDefinitionId.Scope)
	}

	// "Updating" a role definition actually creates a new one and these get consolidated a few seconds later
	// where the "create date" and "update date" match for the newly created record
	// but eventually switch to being the old create date and the new update date
	// ergo we can can for the old create date and the new updated date
	log.Printf("[DEBUG] Waiting for Role Definition %q (Scope %q) to settle down..", roleDefinitionId.RoleID, roleDefinitionId.Scope)
	stateConf := &resource.StateChangeConf{
		ContinuousTargetOccurence: 5,
		Delay:                     10 * time.Second,
		MinTimeout:                10 * time.Second,
		Pending:                   []string{"Pending"},
		Target:                    []string{"Updated"},
		Refresh:                   roleDefinitionEventualConsistencyUpdate(ctx, client, *roleDefinitionId, *updatedOn),
		Timeout:                   d.Timeout(schema.TimeoutUpdate),
	}
	if _, err := stateConf.WaitForState(); err != nil {
		return fmt.Errorf("waiting for Role Definition %q (Scope %q) to settle down: %+v", roleDefinitionId.RoleID, roleDefinitionId.Scope, err)
	}

	return resourceArmRoleDefinitionRead(d, meta)
}

func resourceArmRoleDefinitionRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Authorization.RoleDefinitionsClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	roleDefinitionId, err := parse.RoleDefinitionId(d.Id())
	if err != nil {
		return err
	}

	d.Set("scope", roleDefinitionId.Scope)
	d.Set("role_definition_id", roleDefinitionId.RoleID)
	d.Set("role_definition_resource_id", roleDefinitionId.ResourceID)

	resp, err := client.Get(ctx, roleDefinitionId.Scope, roleDefinitionId.RoleID)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			log.Printf("[DEBUG] Role Definition %q was not found - removing from state", d.Id())
			d.SetId("")
			return nil
		}

		return fmt.Errorf("Error loading Role Definition %q: %+v", d.Id(), err)
	}

	if props := resp.RoleDefinitionProperties; props != nil {
		d.Set("name", props.RoleName)
		d.Set("description", props.Description)

		permissions := flattenRoleDefinitionPermissions(props.Permissions)
		if err := d.Set("permissions", permissions); err != nil {
			return err
		}

		assignableScopes := flattenRoleDefinitionAssignableScopes(props.AssignableScopes)
		if err := d.Set("assignable_scopes", assignableScopes); err != nil {
			return err
		}
	}

	return nil
}

func resourceArmRoleDefinitionDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Authorization.RoleDefinitionsClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, _ := parse.RoleDefinitionId(d.Id())

	resp, err := client.Delete(ctx, id.Scope, id.RoleID)
	if err != nil {
		if !utils.ResponseWasNotFound(resp.Response) {
			return fmt.Errorf("deleting Role Definition %q at Scope %q: %+v", id.RoleID, id.Scope, err)
		}
	}

	return nil
}

func roleDefinitionEventualConsistencyUpdate(ctx context.Context, client azuresdkhacks.RoleDefinitionsWorkaroundClient, id parse.RoleDefinitionID, expectedUpdateDate string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		resp, err := client.Get(ctx, id.Scope, id.RoleID)
		if err != nil {
			return resp, "Failed", err
		}
		if resp.RoleDefinitionProperties == nil {
			return resp, "Failed", fmt.Errorf("`properties` was nil")
		}
		if resp.RoleDefinitionProperties.CreatedOn == nil {
			return resp, "Failed", fmt.Errorf("`properties.CreatedOn` was nil")
		}

		respCreatedOn := *resp.RoleDefinitionProperties.CreatedOn
		respUpdatedOn := *resp.RoleDefinitionProperties.UpdatedOn
		if respCreatedOn == expectedUpdateDate {
			// a new role definition is created and eventually (~5s) reconciled
			return resp, "Pending", nil
		}
		if respUpdatedOn != expectedUpdateDate {
			// however the updatedOn should match the new date, to show this has been reconciled
			return resp, "Pending", nil
		}

		return resp, "Updated", nil
	}
}

func expandRoleDefinitionPermissions(input []interface{}) []authorization.Permission {
	output := make([]authorization.Permission, 0)
	if len(input) == 0 {
		return output
	}

	for _, v := range input {
		if v == nil {
			continue
		}

		raw := v.(map[string]interface{})
		permission := authorization.Permission{}

		actionsOutput := make([]string, 0)
		actions := raw["actions"].([]interface{})
		for _, a := range actions {
			if a == nil {
				continue
			}
			actionsOutput = append(actionsOutput, a.(string))
		}
		permission.Actions = &actionsOutput

		dataActionsOutput := make([]string, 0)
		dataActions := raw["data_actions"].(*schema.Set)
		for _, a := range dataActions.List() {
			if a == nil {
				continue
			}
			dataActionsOutput = append(dataActionsOutput, a.(string))
		}
		permission.DataActions = &dataActionsOutput

		notActionsOutput := make([]string, 0)
		notActions := raw["not_actions"].([]interface{})
		for _, a := range notActions {
			if a == nil {
				continue
			}
			notActionsOutput = append(notActionsOutput, a.(string))
		}
		permission.NotActions = &notActionsOutput

		notDataActionsOutput := make([]string, 0)
		notDataActions := raw["not_data_actions"].(*schema.Set)
		for _, a := range notDataActions.List() {
			if a == nil {
				continue
			}
			notDataActionsOutput = append(notDataActionsOutput, a.(string))
		}
		permission.NotDataActions = &notDataActionsOutput

		output = append(output, permission)
	}

	return output
}

func expandRoleDefinitionAssignableScopes(d *schema.ResourceData) []string {
	scopes := make([]string, 0)

	// The first scope in the list must be the target scope as it it not returned in any API call
	assignedScope := d.Get("scope").(string)
	scopes = append(scopes, assignedScope)
	assignableScopes := d.Get("assignable_scopes").([]interface{})
	for _, scope := range assignableScopes {
		// Ensure the assigned scope is not duplicated in the list if also specified in `assignable_scopes`
		if scope != assignedScope {
			scopes = append(scopes, scope.(string))
		}
	}

	return scopes
}

func flattenRoleDefinitionPermissions(input *[]authorization.Permission) []interface{} {
	permissions := make([]interface{}, 0)
	if input == nil {
		return permissions
	}

	for _, permission := range *input {
		permissions = append(permissions, map[string]interface{}{
			"actions":          utils.FlattenStringSlice(permission.Actions),
			"data_actions":     schema.NewSet(schema.HashString, utils.FlattenStringSlice(permission.DataActions)),
			"not_actions":      utils.FlattenStringSlice(permission.NotActions),
			"not_data_actions": schema.NewSet(schema.HashString, utils.FlattenStringSlice(permission.NotDataActions)),
		})
	}

	return permissions
}

func flattenRoleDefinitionAssignableScopes(input *[]string) []interface{} {
	scopes := make([]interface{}, 0)
	if input == nil {
		return scopes
	}

	for _, scope := range *input {
		scopes = append(scopes, scope)
	}

	return scopes
}

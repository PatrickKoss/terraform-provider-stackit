package mariadb

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	mock_instance "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/mariadb/instance/mock"
	"github.com/stackitcloud/stackit-sdk-go/core/utils"
	"github.com/stackitcloud/stackit-sdk-go/services/mariadb"
	"go.uber.org/mock/gomock"
)

// TestContext holds common test setup
type TestContext struct {
	T          *testing.T
	MockCtrl   *gomock.Controller
	MockClient *mock_instance.MockDefaultApi
	Resource   *instanceResource
	Ctx        context.Context
	CancelFunc context.CancelFunc
}

// NewTestContext creates a new test context with mock client
func NewTestContext(t *testing.T) *TestContext {
	ctrl := gomock.NewController(t)
	mockClient := mock_instance.NewMockDefaultApi(ctrl)

	resource := &instanceResource{
		client: mockClient,
	}

	return &TestContext{
		T:          t,
		MockCtrl:   ctrl,
		MockClient: mockClient,
		Resource:   resource,
		Ctx:        context.Background(),
	}
}

// Close cleans up the test context
func (tc *TestContext) Close() {
	if tc.CancelFunc != nil {
		tc.CancelFunc()
	}
	tc.MockCtrl.Finish()
}

// GetSchema returns the resource schema
func (tc *TestContext) GetSchema() resource.SchemaResponse {
	schemaResp := resource.SchemaResponse{}
	tc.Resource.Schema(tc.Ctx, resource.SchemaRequest{}, &schemaResp)
	return schemaResp
}

// BuildListOfferingsResponse creates a ListOfferingsResponse with a single plan
func BuildListOfferingsResponse(version, planId, planName string) *mariadb.ListOfferingsResponse {
	return &mariadb.ListOfferingsResponse{
		Offerings: &[]mariadb.Offering{
			{
				Version: utils.Ptr(version),
				Plans: &[]mariadb.Plan{
					{
						Id:   utils.Ptr(planId),
						Name: utils.Ptr(planName),
					},
				},
			},
		},
	}
}

// BuildListOfferingsResponseWithMultiplePlans creates a ListOfferingsResponse with multiple plans
func BuildListOfferingsResponseWithMultiplePlans(version string, plans map[string]string) *mariadb.ListOfferingsResponse {
	var planList []mariadb.Plan
	for planId, planName := range plans {
		planList = append(planList, mariadb.Plan{
			Id:   utils.Ptr(planId),
			Name: utils.Ptr(planName),
		})
	}

	return &mariadb.ListOfferingsResponse{
		Offerings: &[]mariadb.Offering{
			{
				Version: utils.Ptr(version),
				Plans:   &planList,
			},
		},
	}
}

// BuildInstance creates a mariadb.Instance with the given fields
func BuildInstance(instanceId, name, planId, dashboardUrl string) *mariadb.Instance {
	return &mariadb.Instance{
		InstanceId:         utils.Ptr(instanceId),
		Name:               utils.Ptr(name),
		PlanId:             utils.Ptr(planId),
		DashboardUrl:       utils.Ptr(dashboardUrl),
		CfGuid:             utils.Ptr("cf-guid"),
		CfSpaceGuid:        utils.Ptr("cf-space-guid"),
		CfOrganizationGuid: utils.Ptr("cf-org-guid"),
		ImageUrl:           utils.Ptr("https://image.example.com"),
		Parameters:         &map[string]interface{}{},
		Status:             mariadb.INSTANCESTATUS_ACTIVE.Ptr(), // Wait handler checks for status
	}
}

// BuildCreateInstanceResponse creates a CreateInstanceResponse
func BuildCreateInstanceResponse(instanceId string) *mariadb.CreateInstanceResponse {
	return &mariadb.CreateInstanceResponse{
		InstanceId: utils.Ptr(instanceId),
	}
}

// CreateTestModel creates a test model with common values
func CreateTestModel(projectId, instanceId, name, version, planName string) Model {
	return Model{
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(name),
		Version:    types.StringValue(version),
		PlanName:   types.StringValue(planName),
		Parameters: types.ObjectNull(parametersTypes),
	}
}

// CreateRequest creates a test Create request
func CreateRequest(ctx context.Context, schema resource.SchemaResponse, model Model) resource.CreateRequest {
	req := resource.CreateRequest{}
	req.Plan = tfsdk.Plan{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.Plan.Set(ctx, model)
	return req
}

// CreateResponse creates a test Create response
func CreateResponse(schema resource.SchemaResponse) *resource.CreateResponse {
	resp := &resource.CreateResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	return resp
}

// UpdateRequest creates a test Update request
func UpdateRequest(ctx context.Context, schema resource.SchemaResponse, currentState, plannedState Model) resource.UpdateRequest {
	req := resource.UpdateRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.Plan = tfsdk.Plan{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, currentState)
	req.Plan.Set(ctx, plannedState)
	return req
}

// UpdateResponse creates a test Update response
// Optionally initialize with current state to simulate Terraform framework behavior
func UpdateResponse(ctx context.Context, schema resource.SchemaResponse, currentState *Model) *resource.UpdateResponse {
	resp := &resource.UpdateResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	// Initialize with current state to simulate framework behavior
	// When Update errors without calling State.Set(), this state is preserved
	if currentState != nil {
		resp.State.Set(ctx, *currentState)
	}
	return resp
}

// DeleteRequest creates a test Delete request
func DeleteRequest(ctx context.Context, schema resource.SchemaResponse, state Model) resource.DeleteRequest {
	req := resource.DeleteRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, state)
	return req
}

// DeleteResponse creates a test Delete response
// Optionally initialize with current state to simulate Terraform framework behavior
func DeleteResponse(ctx context.Context, schema resource.SchemaResponse, currentState *Model) *resource.DeleteResponse {
	resp := &resource.DeleteResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	// Initialize with current state to simulate framework behavior
	// When Delete errors without calling State.RemoveResource(), this state is preserved
	if currentState != nil {
		resp.State.Set(ctx, *currentState)
	}
	return resp
}

// ReadRequest creates a test Read request
func ReadRequest(ctx context.Context, schema resource.SchemaResponse, state Model) resource.ReadRequest {
	req := resource.ReadRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, state)
	return req
}

// ReadResponse creates a test Read response
func ReadResponse(schema resource.SchemaResponse) *resource.ReadResponse {
	resp := &resource.ReadResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	return resp
}

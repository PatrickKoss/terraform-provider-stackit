package mariadb

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stackitcloud/stackit-sdk-go/core/oapierror"
	"github.com/stackitcloud/stackit-sdk-go/services/mariadb"
	mock_instance "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/mariadb/instance/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreate_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "instance-abc-123"
	instanceName := "my-test-instance"
	planId := "plan-123"
	planName := "stackit-mariadb-1.4.10-single"
	version := "1.4.10"

	// Setup mock expectations
	offeringsResp := BuildListOfferingsResponse(version, planId, planName)
	createResp := BuildCreateInstanceResponse(instanceId)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Mock ListOfferings for loadPlanId (called during both Create and Read operations)
	mockListOfferingsReq := mock_instance.NewMockApiListOfferingsRequest(tc.MockCtrl)
	mockListOfferingsReq.EXPECT().
		Execute().
		Return(offeringsResp, nil).
		Times(2)

	tc.MockClient.EXPECT().
		ListOfferings(gomock.Any(), projectId).
		Return(mockListOfferingsReq).
		Times(2)

	// Mock CreateInstance (resource.go line 314)
	mockCreateReq := mock_instance.NewMockApiCreateInstanceRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateInstancePayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(createResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		CreateInstance(gomock.Any(), projectId).
		Return(mockCreateReq).
		Times(1)

	// Mock GetInstanceExecute for wait handler (wait handler calls this directly, not fluent API)
	tc.MockClient.EXPECT().
		GetInstanceExecute(gomock.Any(), projectId, instanceId).
		DoAndReturn(func(ctx context.Context, projectId, instanceId string) (*mariadb.Instance, error) {
			time.Sleep(150 * time.Millisecond) // Longer than context timeout
			return nil, ctx.Err()
		}).
		AnyTimes()

	// Build complete instance response for subsequent Read operation
	dashboardUrl := "https://dashboard.example.com"
	completeInstance := BuildInstance(instanceId, instanceName, planId, dashboardUrl)

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, "", instanceName, version, planName)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	// Assertions
	require.False(t, resp.Diagnostics.HasError(), "Expected no error due to context timeout")

	var stateAfterCreate Model
	diags := resp.State.Get(tc.Ctx, &stateAfterCreate)
	require.False(t, diags.HasError(), "Expected no errors reading state")

	// Verify idempotency - InstanceId should be saved immediately after CreateInstance succeeds
	require.False(t, stateAfterCreate.InstanceId.IsNull(), "BUG: InstanceId should be saved to state immediately after CreateInstance API succeeds")
	require.NotEmpty(t, stateAfterCreate.InstanceId.ValueString(), "InstanceId should not be empty")

	// Verify basic fields from input/CreateInstance response are set
	require.Equal(t, instanceId, stateAfterCreate.InstanceId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s", projectId, instanceId), stateAfterCreate.Id.ValueString())
	require.Equal(t, projectId, stateAfterCreate.ProjectId.ValueString())
	require.Equal(t, instanceName, stateAfterCreate.Name.ValueString())
	require.Equal(t, version, stateAfterCreate.Version.ValueString())
	require.Equal(t, planName, stateAfterCreate.PlanName.ValueString())
	require.Equal(t, planId, stateAfterCreate.PlanId.ValueString())

	// CRITICAL: Verify fields that require GetInstance are NULL after failed wait
	// If these are not null, it means wait succeeded (which shouldn't happen with our timeout)
	// If Read doesn't populate these, Terraform will detect state drift
	require.True(t, stateAfterCreate.DashboardUrl.IsNull(), "DashboardUrl should be null after failed wait (only GetInstance provides this)")
	require.True(t, stateAfterCreate.CfGuid.IsNull(), "CfGuid should be null after failed wait (only GetInstance provides this)")
	require.True(t, stateAfterCreate.ImageUrl.IsNull(), "ImageUrl should be null after failed wait (only GetInstance provides this)")

	// Simulate the next Terraform run: Read operation with the partial state from failed Create
	// This tests the idempotency guarantee - Read should fill in missing fields without errors
	// Setup mock for subsequent Read operation (uses fluent API GetInstance)
	mockGetReq := mock_instance.NewMockApiGetInstanceRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(completeInstance, nil).
		Times(1)

	tc.MockClient.EXPECT().
		GetInstance(gomock.Any(), projectId, instanceId).
		Return(mockGetReq).
		Times(1)

	// Pass the partial state from Create to Read (simulating Terraform's behavior)
	readReq := ReadRequest(tc.Ctx, schema, stateAfterCreate)
	readResp := ReadResponse(schema)

	// Execute Read with partial state
	tc.Resource.Read(tc.Ctx, readReq, readResp)

	require.False(t, readResp.Diagnostics.HasError(), "Expected no error during Read operation")

	// Verify that Read successfully populated all fields from the API
	var stateAfterRead Model
	diags = readResp.State.Get(tc.Ctx, &stateAfterRead)
	require.False(t, diags.HasError(), "Expected no errors reading state after Read")

	// Verify all fields are now complete after successful Read (prevents state drift)
	require.Equal(t, instanceId, stateAfterRead.InstanceId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s", projectId, instanceId), stateAfterRead.Id.ValueString())
	require.Equal(t, projectId, stateAfterRead.ProjectId.ValueString())
	require.Equal(t, instanceName, stateAfterRead.Name.ValueString())
	require.Equal(t, planId, stateAfterRead.PlanId.ValueString())
	require.Equal(t, planName, stateAfterRead.PlanName.ValueString())
	require.Equal(t, version, stateAfterRead.Version.ValueString())

	// CRITICAL: Verify fields that were NULL after Create are now populated
	// This prevents Terraform state drift on the next apply
	require.False(t, stateAfterRead.DashboardUrl.IsNull(), "DashboardUrl must be populated by Read to prevent state drift")
	require.Equal(t, dashboardUrl, stateAfterRead.DashboardUrl.ValueString())
	require.False(t, stateAfterRead.CfGuid.IsNull(), "CfGuid must be populated by Read to prevent state drift")
	require.False(t, stateAfterRead.ImageUrl.IsNull(), "ImageUrl must be populated by Read to prevent state drift")
}

func TestCreate_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "instance-abc-123"
	instanceName := "my-test-instance"
	planId := "plan-123"
	planName := "stackit-mariadb-1.4.10-single"
	version := "1.4.10"
	dashboardUrl := "https://dashboard.example.com"

	// Setup mock expectations
	offeringsResp := BuildListOfferingsResponse(version, planId, planName)
	createResp := BuildCreateInstanceResponse(instanceId)
	instanceResp := BuildInstance(instanceId, instanceName, planId, dashboardUrl)

	// Mock ListOfferings for loadPlanId (resource.go line 736)
	mockListOfferingsReq := mock_instance.NewMockApiListOfferingsRequest(tc.MockCtrl)
	mockListOfferingsReq.EXPECT().
		Execute().
		Return(offeringsResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		ListOfferings(gomock.Any(), projectId).
		Return(mockListOfferingsReq).
		Times(1)

	// Mock CreateInstance (resource.go line 314)
	mockCreateReq := mock_instance.NewMockApiCreateInstanceRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateInstancePayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(createResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		CreateInstance(gomock.Any(), projectId).
		Return(mockCreateReq).
		Times(1)

	// Mock GetInstanceExecute for wait handler (wait handler calls this directly, not fluent API)
	tc.MockClient.EXPECT().
		GetInstanceExecute(gomock.Any(), projectId, instanceId).
		Return(instanceResp, nil).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, "", instanceName, version, planName)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Create should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify all fields match what was returned from GetInstance
	require.Equal(t, instanceId, finalState.InstanceId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s", projectId, instanceId), finalState.Id.ValueString())
	require.Equal(t, projectId, finalState.ProjectId.ValueString())
	require.Equal(t, instanceName, finalState.Name.ValueString())
	require.Equal(t, planId, finalState.PlanId.ValueString())
	require.Equal(t, planName, finalState.PlanName.ValueString())
	require.Equal(t, version, finalState.Version.ValueString())
	require.Equal(t, dashboardUrl, finalState.DashboardUrl.ValueString())

	require.False(t, finalState.CfGuid.IsNull(), "CfGuid should be set from API response")
	require.False(t, finalState.ImageUrl.IsNull(), "ImageUrl should be set from API response")
}

func TestCreate_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	planId := "plan-123"
	planName := "stackit-mariadb-1.4.10-single"
	version := "1.4.10"

	// Setup mock expectations
	offeringsResp := BuildListOfferingsResponse(version, planId, planName)
	apiErr := &oapierror.GenericOpenAPIError{}

	// Mock ListOfferings for loadPlanId (resource.go line 736)
	mockListOfferingsReq := mock_instance.NewMockApiListOfferingsRequest(tc.MockCtrl)
	mockListOfferingsReq.EXPECT().
		Execute().
		Return(offeringsResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		ListOfferings(gomock.Any(), projectId).
		Return(mockListOfferingsReq).
		Times(1)

	// Mock CreateInstance to fail (resource.go line 314)
	mockCreateReq := mock_instance.NewMockApiCreateInstanceRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateInstancePayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(nil, apiErr).
		Times(1)

	tc.MockClient.EXPECT().
		CreateInstance(gomock.Any(), projectId).
		Return(mockCreateReq).
		Times(1)

	schema := tc.GetSchema()
	model := CreateTestModel(projectId, "", "test-instance", version, planName)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	require.True(t, resp.Diagnostics.HasError(), "Expected error when API call fails")

	// State should be empty since instance was never created
	var stateAfterCreate Model
	resp.State.Get(tc.Ctx, &stateAfterCreate)

	require.True(t, stateAfterCreate.InstanceId.IsNull(), "InstanceId should be null when API call fails")
}

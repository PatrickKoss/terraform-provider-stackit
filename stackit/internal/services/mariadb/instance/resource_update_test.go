package mariadb

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stackitcloud/stackit-sdk-go/core/oapierror"
	"github.com/stackitcloud/stackit-sdk-go/services/mariadb"
	mock_instance "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/mariadb/instance/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestUpdate_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "instance-abc-123"
	instanceName := "my-test-instance"
	oldPlanId := "old-plan-123"
	newPlanId := "new-plan-456"
	oldPlanName := "stackit-mariadb-1.4.10-single"
	newPlanName := "stackit-mariadb-1.4.10-replica"
	version := "1.4.10"
	dashboardUrl := "https://dashboard.example.com"

	// Setup mock expectations
	plans := map[string]string{
		oldPlanId: oldPlanName,
		newPlanId: newPlanName,
	}
	offeringsResp := BuildListOfferingsResponseWithMultiplePlans(version, plans)
	instanceResp := BuildInstance(instanceId, instanceName, newPlanId, dashboardUrl)

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

	// Mock PartialUpdateInstance (resource.go line 456)
	mockUpdateReq := mock_instance.NewMockApiPartialUpdateInstanceRequest(tc.MockCtrl)
	mockUpdateReq.EXPECT().
		PartialUpdateInstancePayload(gomock.Any()).
		Return(mockUpdateReq).
		Times(1)
	mockUpdateReq.EXPECT().
		Execute().
		Return(nil).
		Times(1)

	tc.MockClient.EXPECT().
		PartialUpdateInstance(gomock.Any(), projectId, instanceId).
		Return(mockUpdateReq).
		Times(1)

	// Mock GetInstanceExecute for wait handler (wait handler calls this directly, not fluent API)
	tc.MockClient.EXPECT().
		GetInstanceExecute(gomock.Any(), projectId, instanceId).
		Return(instanceResp, nil).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()

	currentState := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		Version:    types.StringValue(version),
		PlanId:     types.StringValue(oldPlanId),
		PlanName:   types.StringValue(oldPlanName),
		Parameters: types.ObjectNull(parametersTypes),
	}

	plannedState := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		Version:    types.StringValue(version),
		PlanId:     types.StringValue(newPlanId),
		PlanName:   types.StringValue(newPlanName),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := UpdateRequest(tc.Ctx, schema, currentState, plannedState)
	resp := UpdateResponse(tc.Ctx, schema, nil)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	require.False(t, resp.Diagnostics.HasError(), "Update should succeed, but got errors: %v", resp.Diagnostics.Errors())

	// Extract final state
	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify all fields match the updated values from GetInstance
	require.Equal(t, instanceId, finalState.InstanceId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s", projectId, instanceId), finalState.Id.ValueString())
	require.Equal(t, projectId, finalState.ProjectId.ValueString())
	require.Equal(t, instanceName, finalState.Name.ValueString())
	require.Equal(t, newPlanId, finalState.PlanId.ValueString())
	require.Equal(t, newPlanName, finalState.PlanName.ValueString())
	require.Equal(t, version, finalState.Version.ValueString())
	require.Equal(t, dashboardUrl, finalState.DashboardUrl.ValueString())
}

func TestUpdate_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "instance-abc-123"
	instanceName := "my-test-instance"
	oldPlanId := "old-plan-123"
	newPlanId := "new-plan-456"
	oldPlanName := "stackit-mariadb-1.4.10-single"
	newPlanName := "stackit-mariadb-1.4.10-replica"
	version := "1.4.10"

	// Setup mock expectations
	plans := map[string]string{
		oldPlanId: oldPlanName,
		newPlanId: newPlanName,
	}
	offeringsResp := BuildListOfferingsResponseWithMultiplePlans(version, plans)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

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

	// Mock PartialUpdateInstance (resource.go line 456)
	mockUpdateReq := mock_instance.NewMockApiPartialUpdateInstanceRequest(tc.MockCtrl)
	mockUpdateReq.EXPECT().
		PartialUpdateInstancePayload(gomock.Any()).
		Return(mockUpdateReq).
		Times(1)
	mockUpdateReq.EXPECT().
		Execute().
		Return(nil).
		Times(1)

	tc.MockClient.EXPECT().
		PartialUpdateInstance(gomock.Any(), projectId, instanceId).
		Return(mockUpdateReq).
		Times(1)

	// Mock GetInstanceExecute for wait handler (wait handler calls this directly, not fluent API)
	tc.MockClient.EXPECT().
		GetInstanceExecute(gomock.Any(), projectId, instanceId).
		DoAndReturn(func(ctx context.Context, projectId, instanceId string) (*mariadb.Instance, error) {
			time.Sleep(150 * time.Millisecond) // Longer than context timeout
			return nil, ctx.Err()
		}).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()

	currentState := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		Version:    types.StringValue(version),
		PlanId:     types.StringValue(oldPlanId),
		PlanName:   types.StringValue(oldPlanName),
		Parameters: types.ObjectNull(parametersTypes),
	}

	plannedState := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		Version:    types.StringValue(version),
		PlanId:     types.StringValue(newPlanId),
		PlanName:   types.StringValue(newPlanName),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := UpdateRequest(tc.Ctx, schema, currentState, plannedState)
	resp := UpdateResponse(tc.Ctx, schema, &currentState)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	require.True(t, resp.Diagnostics.HasError(), "Expected error due to context timeout")

	var stateAfterUpdate Model
	diags := resp.State.Get(tc.Ctx, &stateAfterUpdate)
	require.False(t, diags.HasError(), "Failed to get state after update: %v", diags.Errors())

	// State should preserve old values since update wait failed
	if !stateAfterUpdate.PlanId.IsNull() {
		actualPlanId := stateAfterUpdate.PlanId.ValueString()
		require.NotEqual(t, newPlanId, actualPlanId, "BUG: State has NEW PlanId even though update wait failed!")
		require.Equal(t, oldPlanId, actualPlanId)
	}

	if !stateAfterUpdate.PlanName.IsNull() {
		actualPlanName := stateAfterUpdate.PlanName.ValueString()
		require.NotEqual(t, newPlanName, actualPlanName, "BUG: State has NEW PlanName even though update wait failed!")
		require.Equal(t, oldPlanName, actualPlanName)
	}

	require.Equal(t, currentState.Id.ValueString(), stateAfterUpdate.Id.ValueString())
	require.Equal(t, currentState.ProjectId.ValueString(), stateAfterUpdate.ProjectId.ValueString())
	require.Equal(t, currentState.InstanceId.ValueString(), stateAfterUpdate.InstanceId.ValueString())
	require.Equal(t, currentState.Name.ValueString(), stateAfterUpdate.Name.ValueString())
	require.Equal(t, currentState.Version.ValueString(), stateAfterUpdate.Version.ValueString())
}

func TestUpdate_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	instanceId := "test-instance"

	// Setup mock expectations
	offeringsResp := BuildListOfferingsResponseWithMultiplePlans("1.4.10", map[string]string{
		"old-plan": "old-plan-name",
		"new-plan": "new-plan-name",
	})
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

	// Mock PartialUpdateInstance to fail (resource.go line 456)
	mockUpdateReq := mock_instance.NewMockApiPartialUpdateInstanceRequest(tc.MockCtrl)
	mockUpdateReq.EXPECT().
		PartialUpdateInstancePayload(gomock.Any()).
		Return(mockUpdateReq).
		Times(1)
	mockUpdateReq.EXPECT().
		Execute().
		Return(apiErr).
		Times(1)

	tc.MockClient.EXPECT().
		PartialUpdateInstance(gomock.Any(), projectId, instanceId).
		Return(mockUpdateReq).
		Times(1)

	// Prepare request
	schema := tc.GetSchema()

	currentState := Model{
		Id:         types.StringValue("test-project,test-instance"),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue("test-name"),
		Version:    types.StringValue("1.4.10"),
		PlanId:     types.StringValue("old-plan"),
		PlanName:   types.StringValue("old-plan-name"),
		Parameters: types.ObjectNull(parametersTypes),
	}

	plannedState := Model{
		Id:         types.StringValue("test-project,test-instance"),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue("test-name"),
		Version:    types.StringValue("1.4.10"),
		PlanId:     types.StringValue("new-plan"),
		PlanName:   types.StringValue("new-plan-name"),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := UpdateRequest(tc.Ctx, schema, currentState, plannedState)
	resp := UpdateResponse(tc.Ctx, schema, nil)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	require.True(t, resp.Diagnostics.HasError(), "Expected error when API call fails")
}

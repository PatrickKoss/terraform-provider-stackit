package mariadb

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stackitcloud/stackit-sdk-go/core/oapierror"
	mock_instance "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/mariadb/instance/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRead_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	instanceId := "instance-abc-123"
	instanceName := "my-test-instance"
	planId := "plan-123"
	planName := "stackit-mariadb-1.4.10-single"
	version := "1.4.10"
	dashboardUrl := "https://dashboard.example.com"

	// Setup mock expectations
	instanceResp := BuildInstance(instanceId, instanceName, planId, dashboardUrl)
	offeringsResp := BuildListOfferingsResponse(version, planId, planName)

	// Mock GetInstance (resource.go line 386)
	mockGetReq := mock_instance.NewMockApiGetInstanceRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(instanceResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		GetInstance(gomock.Any(), projectId, instanceId).
		Return(mockGetReq).
		Times(1)

	// Mock ListOfferings for loadPlanNameAndVersion (resource.go line 774)
	mockListOfferingsReq := mock_instance.NewMockApiListOfferingsRequest(tc.MockCtrl)
	mockListOfferingsReq.EXPECT().
		Execute().
		Return(offeringsResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		ListOfferings(gomock.Any(), projectId).
		Return(mockListOfferingsReq).
		Times(1)

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		// Other fields may be outdated or null
		Name:       types.StringNull(),
		Version:    types.StringNull(),
		PlanId:     types.StringNull(),
		PlanName:   types.StringNull(),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var refreshedState Model
	diags := resp.State.Get(tc.Ctx, &refreshedState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	require.Equal(t, instanceId, refreshedState.InstanceId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s", projectId, instanceId), refreshedState.Id.ValueString())
	require.Equal(t, projectId, refreshedState.ProjectId.ValueString())
	require.Equal(t, instanceName, refreshedState.Name.ValueString())
	require.Equal(t, planId, refreshedState.PlanId.ValueString())
	require.Equal(t, planName, refreshedState.PlanName.ValueString())
	require.Equal(t, version, refreshedState.Version.ValueString())
	require.Equal(t, dashboardUrl, refreshedState.DashboardUrl.ValueString())

	require.False(t, refreshedState.CfGuid.IsNull(), "CfGuid should be set from API response")
	require.False(t, refreshedState.ImageUrl.IsNull(), "ImageUrl should be set from API response")
}

func TestRead_InstanceNotFound(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	instanceId := "non-existent-instance"

	// Setup GetInstance to return 404
	notFoundErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusNotFound,
	}

	// Mock GetInstance (resource.go line 386)
	mockGetReq := mock_instance.NewMockApiGetInstanceRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(nil, notFoundErr).
		Times(1)

	tc.MockClient.EXPECT().
		GetInstance(gomock.Any(), projectId, instanceId).
		Return(mockGetReq).
		Times(1)

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should not error when instance not found, but got errors: %v", resp.Diagnostics.Errors())
}

func TestRead_InstanceGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	instanceId := "gone-instance"

	// Setup GetInstance to return 410 Gone
	goneErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusGone,
	}

	// Mock GetInstance (resource.go line 386)
	mockGetReq := mock_instance.NewMockApiGetInstanceRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(nil, goneErr).
		Times(1)

	tc.MockClient.EXPECT().
		GetInstance(gomock.Any(), projectId, instanceId).
		Return(mockGetReq).
		Times(1)

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should not error when instance is gone, but got errors: %v", resp.Diagnostics.Errors())
}

func TestRead_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	instanceId := "test-instance"

	// Setup GetInstance to return 500 error
	serverErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusInternalServerError,
	}

	// Mock GetInstance (resource.go line 386)
	mockGetReq := mock_instance.NewMockApiGetInstanceRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(nil, serverErr).
		Times(1)

	tc.MockClient.EXPECT().
		GetInstance(gomock.Any(), projectId, instanceId).
		Return(mockGetReq).
		Times(1)

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue("test-project,test-instance"),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.True(t, resp.Diagnostics.HasError(), "Expected error when API call fails")
}

func TestRead_DetectDrift(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	instanceId := "instance-abc-123"
	instanceName := "my-test-instance"
	oldPlanId := "old-plan-123"
	newPlanId := "new-plan-456" // Changed in cloud
	oldPlanName := "stackit-mariadb-1.4.10-single"
	newPlanName := "stackit-mariadb-1.4.10-replica" // Changed in cloud
	version := "1.4.10"
	dashboardUrl := "https://dashboard.example.com"

	// Setup mock expectations - instance has drifted to new plan
	instanceResp := BuildInstance(instanceId, instanceName, newPlanId, dashboardUrl)
	plans := map[string]string{
		oldPlanId: oldPlanName,
		newPlanId: newPlanName,
	}
	offeringsResp := BuildListOfferingsResponseWithMultiplePlans(version, plans)

	// Mock GetInstance - returns drifted state (resource.go line 386)
	mockGetReq := mock_instance.NewMockApiGetInstanceRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(instanceResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		GetInstance(gomock.Any(), projectId, instanceId).
		Return(mockGetReq).
		Times(1)

	// Mock ListOfferings for loadPlanNameAndVersion (resource.go line 774)
	mockListOfferingsReq := mock_instance.NewMockApiListOfferingsRequest(tc.MockCtrl)
	mockListOfferingsReq.EXPECT().
		Execute().
		Return(offeringsResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		ListOfferings(gomock.Any(), projectId).
		Return(mockListOfferingsReq).
		Times(1)

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		Version:    types.StringValue(version),
		PlanId:     types.StringValue(oldPlanId),   // Old value
		PlanName:   types.StringValue(oldPlanName), // Old value
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var refreshedState Model
	diags := resp.State.Get(tc.Ctx, &refreshedState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify drift was detected and state was updated
	require.Equal(t, newPlanId, refreshedState.PlanId.ValueString())
	require.Equal(t, newPlanName, refreshedState.PlanName.ValueString())
}

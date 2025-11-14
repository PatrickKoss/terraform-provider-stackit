package mariadb

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stackitcloud/stackit-sdk-go/core/oapierror"
	"github.com/stackitcloud/stackit-sdk-go/services/mariadb"
	mock_instance "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/mariadb/instance/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDelete_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	instanceId := "instance-abc-123"
	instanceName := "my-test-instance"
	planId := "plan-123"
	planName := "stackit-mariadb-1.4.10-single"
	version := "1.4.10"

	// Setup mock expectations
	goneErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusGone,
	}

	// Mock DeleteInstance (resource.go line 504)
	mockDeleteReq := mock_instance.NewMockApiDeleteInstanceRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(nil).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteInstance(gomock.Any(), projectId, instanceId).
		Return(mockDeleteReq).
		Times(1)

	// Mock GetInstanceExecute for wait handler (wait handler calls this directly, not fluent API)
	tc.MockClient.EXPECT().
		GetInstanceExecute(gomock.Any(), projectId, instanceId).
		Return(nil, goneErr).
		AnyTimes()

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		Version:    types.StringValue(version),
		PlanId:     types.StringValue(planId),
		PlanName:   types.StringValue(planName),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Delete should succeed, but got errors: %v", resp.Diagnostics.Errors())
}

func TestDelete_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	instanceId := "instance-abc-123"
	instanceName := "my-test-instance"
	planId := "plan-123"
	planName := "stackit-mariadb-1.4.10-single"
	version := "1.4.10"

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Mock DeleteInstance (resource.go line 504)
	mockDeleteReq := mock_instance.NewMockApiDeleteInstanceRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(nil).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteInstance(gomock.Any(), projectId, instanceId).
		Return(mockDeleteReq).
		Times(1)

	// Mock GetInstanceExecute for wait handler (wait handler calls this directly, not fluent API)
	tc.MockClient.EXPECT().
		GetInstanceExecute(gomock.Any(), projectId, instanceId).
		DoAndReturn(func(ctx context.Context, projectId, instanceId string) (*mariadb.Instance, error) {
			time.Sleep(150 * time.Millisecond) // Longer than context timeout
			return nil, ctx.Err()
		}).
		AnyTimes()

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		Version:    types.StringValue(version),
		PlanId:     types.StringValue(planId),
		PlanName:   types.StringValue(planName),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, &state)

	tc.Resource.Delete(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Expected no error due to context timeout")

	var stateAfterDelete Model
	diags := resp.State.Get(tc.Ctx, &stateAfterDelete)
	require.False(t, diags.HasError(), "Failed to get state after delete: %v", diags.Errors())

	// State should be preserved since delete wait failed
	require.Equal(t, state.InstanceId.ValueString(), stateAfterDelete.InstanceId.ValueString())
	require.Equal(t, state.Id.ValueString(), stateAfterDelete.Id.ValueString())
	require.Equal(t, state.ProjectId.ValueString(), stateAfterDelete.ProjectId.ValueString())
	require.Equal(t, state.Name.ValueString(), stateAfterDelete.Name.ValueString())
	require.Equal(t, state.Version.ValueString(), stateAfterDelete.Version.ValueString())
	require.Equal(t, state.PlanId.ValueString(), stateAfterDelete.PlanId.ValueString())
	require.Equal(t, state.PlanName.ValueString(), stateAfterDelete.PlanName.ValueString())
}

func TestDelete_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	instanceId := "test-instance"

	// Setup mock expectations
	serverErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusInternalServerError,
	}

	// Mock DeleteInstance (resource.go line 504)
	mockDeleteReq := mock_instance.NewMockApiDeleteInstanceRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(serverErr).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteInstance(gomock.Any(), projectId, instanceId).
		Return(mockDeleteReq).
		Times(1)

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue("test-project,test-instance"),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue("test-name"),
		Version:    types.StringValue("1.4.10"),
		PlanId:     types.StringValue("plan-123"),
		PlanName:   types.StringValue("plan-name"),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	require.True(t, resp.Diagnostics.HasError(), "Expected error when API call fails")
}

func TestDelete_InstanceAlreadyDeleted(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	instanceId := "test-instance"

	// Setup mock expectations - DeleteInstance returns 404
	notFoundErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusNotFound,
	}

	// Mock DeleteInstance (resource.go line 504)
	mockDeleteReq := mock_instance.NewMockApiDeleteInstanceRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(notFoundErr).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteInstance(gomock.Any(), projectId, instanceId).
		Return(mockDeleteReq).
		Times(1)

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue("test-project,test-instance"),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue("test-name"),
		Version:    types.StringValue("1.4.10"),
		PlanId:     types.StringValue("plan-123"),
		PlanName:   types.StringValue("plan-name"),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	// Delete should succeed for idempotency - instance already deleted
	require.False(t, resp.Diagnostics.HasError(), "Delete should succeed when instance is already deleted (404), but got errors: %v", resp.Diagnostics.Errors())
}

func TestDelete_InstanceGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	instanceId := "test-instance"

	// Setup mock expectations - DeleteInstance returns 410 Gone
	goneErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusGone,
	}

	// Mock DeleteInstance (resource.go line 504)
	mockDeleteReq := mock_instance.NewMockApiDeleteInstanceRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(goneErr).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteInstance(gomock.Any(), projectId, instanceId).
		Return(mockDeleteReq).
		Times(1)

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue("test-project,test-instance"),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue("test-name"),
		Version:    types.StringValue("1.4.10"),
		PlanId:     types.StringValue("plan-123"),
		PlanName:   types.StringValue("plan-name"),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	// Delete should succeed for idempotency - instance already gone
	require.False(t, resp.Diagnostics.HasError(), "Delete should succeed when instance is gone (410), but got errors: %v", resp.Diagnostics.Errors())
}

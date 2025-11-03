package instance

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestDelete_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "11111111-2222-3333-4444-555555555555"
	instanceId := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	instanceName := "my-test-git"

	// Setup mock handlers
	deleteCalled := 0
	tc.SetupDeleteInstanceHandler(&deleteCalled)
	// Setup Get handler to return 404 (resource deleted successfully)
	tc.SetupGetInstanceHandlerWithStatusCode(http.StatusNotFound)

	// Prepare request with existing instance
	schema := tc.GetSchema()
	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		Version:    types.StringValue("1.0.0"),
		ACL:        types.ListNull(types.StringType),
	}
	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if deleteCalled == 0 {
		t.Fatal("DeleteInstance API should have been called")
	}

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	t.Log("SUCCESS: Instance deleted successfully")
}

func TestDelete_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "11111111-2222-3333-4444-555555555555"
	instanceId := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	instanceName := "my-test-git"

	// Setup mock handlers
	deleteCalled := 0
	tc.SetupDeleteInstanceHandler(&deleteCalled)

	// Setup GetInstance to simulate slow response (triggers timeout)
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		jsonResp := fmt.Sprintf(`{"id": "%s", "name": "%s", "status": "deleting"}`, instanceId, instanceName)
		w.Write([]byte(jsonResp))
	}).Methods("GET")

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Prepare request with existing instance
	schema := tc.GetSchema()
	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		Version:    types.StringValue("1.0.0"),
		ACL:        types.ListNull(types.StringType),
	}
	req := DeleteRequest(tc.Ctx, schema, state)
	// Initialize response with current state (simulates framework preserving state on error)
	resp := DeleteResponse(tc.Ctx, schema, &state)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if deleteCalled == 0 {
		t.Fatal("DeleteInstance API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to context timeout")
	}

	// Verify helpful error message
	errors := resp.Diagnostics.Errors()
	if len(errors) == 0 {
		t.Fatal("Expected error message")
	}
	errorMsg := errors[0].Summary()
	if errorMsg != "Error deleting git instance" {
		t.Errorf("Unexpected error message: %s", errorMsg)
	}

	// After timeout, verify state was NOT removed
	var stateAfterDelete Model
	diags := resp.State.Get(tc.Ctx, &stateAfterDelete)
	if diags.HasError() {
		t.Fatalf("Failed to get state after delete: %v", diags.Errors())
	}

	// Verify ALL fields match original state (resource still tracked)
	AssertStateFieldEquals(t, "InstanceId", stateAfterDelete.InstanceId, state.InstanceId)
	AssertStateFieldEquals(t, "Id", stateAfterDelete.Id, state.Id)
	AssertStateFieldEquals(t, "ProjectId", stateAfterDelete.ProjectId, state.ProjectId)
	AssertStateFieldEquals(t, "Name", stateAfterDelete.Name, state.Name)
	AssertStateFieldEquals(t, "Version", stateAfterDelete.Version, state.Version)

	t.Log("GOOD: State preserved when delete wait fails - user can retry")
}

func TestDelete_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "11111111-2222-3333-4444-555555555555"
	instanceId := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	instanceName := "my-test-git"

	// Setup mock handler to return error
	deleteCalled := 0
	tc.SetupDeleteInstanceHandlerWithStatusCode(500, &deleteCalled)

	// Prepare request with existing instance
	schema := tc.GetSchema()
	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		ACL:        types.ListNull(types.StringType),
	}
	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if deleteCalled == 0 {
		t.Fatal("DeleteInstance API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when API fails")
	}

	t.Log("SUCCESS: Error handled correctly")
}

func TestDelete_ResourceAlreadyDeleted(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "11111111-2222-3333-4444-555555555555"
	instanceId := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	instanceName := "my-test-git"

	// Setup mock handler to return 404 Not Found
	deleteCalled := 0
	tc.SetupDeleteInstanceHandlerWithStatusCode(404, &deleteCalled)

	// Prepare request with existing instance
	schema := tc.GetSchema()
	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		ACL:        types.ListNull(types.StringType),
	}
	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if deleteCalled == 0 {
		t.Fatal("DeleteInstance API should have been called")
	}

	// CRITICAL: Should NOT error - this is idempotency
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed for idempotency (404), but got errors: %v", resp.Diagnostics.Errors())
	}

	t.Log("SUCCESS: Delete is idempotent - succeeds when resource already deleted (404)")
}

func TestDelete_ResourceGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "11111111-2222-3333-4444-555555555555"
	instanceId := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	instanceName := "my-test-git"

	// Setup mock handler to return 410 Gone
	deleteCalled := 0
	tc.SetupDeleteInstanceHandlerWithStatusCode(410, &deleteCalled)

	// Prepare request with existing instance
	schema := tc.GetSchema()
	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		ACL:        types.ListNull(types.StringType),
	}
	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if deleteCalled == 0 {
		t.Fatal("DeleteInstance API should have been called")
	}

	// CRITICAL: Should NOT error - this is idempotency
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed for idempotency (410), but got errors: %v", resp.Diagnostics.Errors())
	}

	t.Log("SUCCESS: Delete is idempotent - succeeds when resource is gone (410)")
}

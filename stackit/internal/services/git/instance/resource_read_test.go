package instance

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRead_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "11111111-2222-3333-4444-555555555555"
	instanceId := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	instanceName := "my-test-git"

	// Setup mock handler
	tc.SetupGetInstanceHandler(instanceId, instanceName, "Ready")

	// Prepare request with existing instance
	schema := tc.GetSchema()
	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue("old-name"), // Will be refreshed
		ACL:        types.ListNull(types.StringType),
	}
	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(tc.Ctx, schema, &state)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Extract state
	var refreshedState Model
	diags := resp.State.Get(tc.Ctx, &refreshedState)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	// Verify all fields match API response
	AssertStateFieldEquals(t, "InstanceId", refreshedState.InstanceId, types.StringValue(instanceId))
	AssertStateFieldEquals(t, "Name", refreshedState.Name, types.StringValue(instanceName))
	AssertStateFieldEquals(t, "ProjectId", refreshedState.ProjectId, types.StringValue(projectId))

	// Verify computed fields
	if refreshedState.Version.IsNull() {
		t.Error("Version should be set from API")
	}
	if refreshedState.Url.IsNull() {
		t.Error("Url should be set from API")
	}

	t.Log("SUCCESS: State refreshed correctly")
}

func TestRead_ResourceNotFound(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "11111111-2222-3333-4444-555555555555"
	instanceId := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	// Setup mock handler to return 404
	tc.SetupGetInstanceHandlerWithStatusCode(http.StatusNotFound)

	// Prepare request with existing instance
	schema := tc.GetSchema()
	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue("my-test-git"),
		ACL:        types.ListNull(types.StringType),
	}
	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(tc.Ctx, schema, &state)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should not error on 404, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Verify state was removed
	var refreshedState Model
	diags := resp.State.Get(tc.Ctx, &refreshedState)

	// State should be empty after removal
	if refreshedState.InstanceId.ValueString() != "" {
		t.Error("State should be removed when resource not found (404)")
	}

	if !diags.HasError() {
		// If no errors, state should be completely empty
		if !refreshedState.Id.IsNull() && refreshedState.Id.ValueString() != "" {
			t.Error("Id should be empty after state removal")
		}
	}

	t.Log("SUCCESS: State removed when resource not found (404)")
}

func TestRead_ResourceGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "11111111-2222-3333-4444-555555555555"
	instanceId := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	// Setup mock handler to return 410 Gone
	tc.SetupGetInstanceHandlerWithStatusCode(http.StatusGone)

	// Prepare request with existing instance
	schema := tc.GetSchema()
	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue("my-test-git"),
		ACL:        types.ListNull(types.StringType),
	}
	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(tc.Ctx, schema, &state)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// The Read function currently only handles 404, not 410
	// This test documents the current behavior
	// Ideally, 410 should also remove the resource from state
	if !resp.Diagnostics.HasError() {
		// If we want to support 410 in the future, we should update Read() to handle it
		t.Log("Note: Read currently doesn't handle 410 Gone - could be improved")
	}
}

func TestRead_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "11111111-2222-3333-4444-555555555555"
	instanceId := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	// Setup mock handler to return 500
	tc.SetupGetInstanceHandlerWithStatusCode(http.StatusInternalServerError)

	// Prepare request with existing instance
	schema := tc.GetSchema()
	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue("my-test-git"),
		ACL:        types.ListNull(types.StringType),
	}
	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(tc.Ctx, schema, &state)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when API fails with 500")
	}

	t.Log("SUCCESS: Error handled correctly")
}

func TestRead_DetectDrift(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "11111111-2222-3333-4444-555555555555"
	instanceId := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	oldName := "old-git-name"
	newName := "new-git-name"

	// Setup mock handler with new name
	tc.SetupGetInstanceHandler(instanceId, newName, "Ready")

	// Prepare request with old state
	schema := tc.GetSchema()
	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(oldName), // Old value
		ACL:        types.ListNull(types.StringType),
	}
	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(tc.Ctx, schema, &state)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Extract state
	var refreshedState Model
	diags := resp.State.Get(tc.Ctx, &refreshedState)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	// Verify state updated with new values (drift detected)
	AssertStateFieldEquals(t, "Name", refreshedState.Name, types.StringValue(newName))
	if refreshedState.Name.ValueString() == oldName {
		t.Error("Drift detection failed - state should have new name")
	}

	t.Log("SUCCESS: Drift detected and state updated")
}

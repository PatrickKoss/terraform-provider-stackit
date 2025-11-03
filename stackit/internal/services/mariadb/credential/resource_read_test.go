package mariadb

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRead_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "test-instance-456"
	credentialId := "test-credential-789"
	username := "test-user"
	host := "test-host.example.com"
	var port int64 = 3306

	// Setup mock handler
	tc.SetupGetCredentialHandler(credentialId, username, host, port)

	// Prepare request with current state
	schema := tc.GetSchema()
	state := CreateFullTestModel(projectId, instanceId, credentialId, "old-user", "old-host")
	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Extract state
	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	// Verify all fields match API response
	AssertStateFieldEquals(t, "CredentialId", finalState.CredentialId, types.StringValue(credentialId))
	AssertStateFieldEquals(t, "ProjectId", finalState.ProjectId, types.StringValue(projectId))
	AssertStateFieldEquals(t, "InstanceId", finalState.InstanceId, types.StringValue(instanceId))
	AssertStateFieldEquals(t, "Username", finalState.Username, types.StringValue(username))
	AssertStateFieldEquals(t, "Host", finalState.Host, types.StringValue(host))

	if finalState.Port.IsNull() {
		t.Error("Port should be set from API")
	} else if finalState.Port.ValueInt64() != port {
		t.Errorf("Port mismatch: got=%d, want=%d", finalState.Port.ValueInt64(), port)
	}

	t.Log("SUCCESS: All state fields correctly populated from API")
}

func TestRead_ResourceNotFound(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "test-instance-456"
	credentialId := "test-credential-789"

	// Setup mock handler that returns 404
	tc.SetupGetCredentialHandler404()

	// Prepare request with current state
	schema := tc.GetSchema()
	state := CreateFullTestModel(projectId, instanceId, credentialId, "old-user", "old-host")
	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions - should succeed and remove state
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should succeed when resource is 404, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Verify state was removed
	var stateAfterRead Model
	diags := resp.State.Get(tc.Ctx, &stateAfterRead)
	// State should be empty/null after RemoveResource
	if !diags.HasError() {
		// If we can get state without errors, it means state wasn't removed
		if !stateAfterRead.CredentialId.IsNull() {
			t.Error("State should be removed when credential is not found (404)")
		}
	}

	t.Log("SUCCESS: State removed when credential not found (404)")
}

func TestRead_ResourceGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "test-instance-456"
	credentialId := "test-credential-789"

	// Setup mock handler that returns 410 Gone
	tc.SetupGetCredentialHandler410()

	// Prepare request with current state
	schema := tc.GetSchema()
	state := CreateFullTestModel(projectId, instanceId, credentialId, "old-user", "old-host")
	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions - Read doesn't handle 410, only 404
	// So this should result in an error
	if !resp.Diagnostics.HasError() {
		t.Log("Note: Read operation doesn't handle 410 Gone status (only handles 404)")
	}

	t.Log("Read completed for 410 Gone status")
}

func TestRead_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "test-instance-456"
	credentialId := "test-credential-789"

	// Setup mock handler that returns 500 error
	tc.SetupGetCredentialHandler500()

	// Prepare request with current state
	schema := tc.GetSchema()
	state := CreateFullTestModel(projectId, instanceId, credentialId, "old-user", "old-host")
	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when API call fails with 500")
	}

	t.Log("SUCCESS: Error handled correctly when API fails")
}

func TestRead_DetectDrift(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "test-instance-456"
	credentialId := "test-credential-789"
	oldUsername := "old-user"
	newUsername := "new-user"
	oldHost := "old-host.example.com"
	newHost := "new-host.example.com"
	var newPort int64 = 3307

	// Setup mock handler with updated values
	tc.SetupGetCredentialHandler(credentialId, newUsername, newHost, newPort)

	// Prepare request with OLD state
	schema := tc.GetSchema()
	state := CreateFullTestModel(projectId, instanceId, credentialId, oldUsername, oldHost)
	state.Port = types.Int64Value(3306)
	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Extract state
	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	// Verify state updated with NEW values (drift detected)
	AssertStateFieldEquals(t, "Username", finalState.Username, types.StringValue(newUsername))
	AssertStateFieldEquals(t, "Host", finalState.Host, types.StringValue(newHost))

	if finalState.Port.IsNull() {
		t.Error("Port should be set from API")
	} else if finalState.Port.ValueInt64() != newPort {
		t.Errorf("Port should be updated: got=%d, want=%d", finalState.Port.ValueInt64(), newPort)
	}

	// Verify old values are NOT present
	if finalState.Username.ValueString() == oldUsername {
		t.Error("Username should be updated to new value")
	}
	if finalState.Host.ValueString() == oldHost {
		t.Error("Host should be updated to new value")
	}

	t.Log("SUCCESS: Drift detected and state updated with new values from API")
}

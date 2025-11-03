package mariadb

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestDelete_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "test-instance-456"
	credentialId := "test-credential-789"

	// Setup mock handlers
	deleteCalled := 0
	tc.SetupDeleteCredentialHandler(&deleteCalled)

	// Setup Get handler to return 410 Gone after deletion
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}/credentials/{credentialId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		w.Write([]byte(`{"error": "gone"}`))
	}).Methods("GET")

	// Prepare request with current state
	schema := tc.GetSchema()
	state := CreateFullTestModel(projectId, instanceId, credentialId, "test-user", "test-host")
	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if deleteCalled == 0 {
		t.Fatal("DeleteCredentials API should have been called")
	}

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	t.Log("SUCCESS: Credential deleted successfully")
}

func TestDelete_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "test-instance-456"
	credentialId := "test-credential-789"

	// Setup mock handlers
	deleteCalled := 0
	tc.SetupDeleteCredentialHandler(&deleteCalled)

	// Setup Get handler to simulate slow response (triggers timeout)
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}/credentials/{credentialId}", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		jsonResp := fmt.Sprintf(`{
			"id": "%s",
			"uri": "mysql://user:pass@host:3306/db"
		}`, credentialId)
		w.Write([]byte(jsonResp))
	}).Methods("GET")

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Prepare request with current state
	schema := tc.GetSchema()
	state := CreateFullTestModel(projectId, instanceId, credentialId, "test-user", "test-host")
	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, &state)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if deleteCalled == 0 {
		t.Fatal("DeleteCredentials API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to context timeout")
	}

	// Verify state is NOT removed (credential still tracked)
	var stateAfterDelete Model
	diags := resp.State.Get(tc.Ctx, &stateAfterDelete)
	if diags.HasError() {
		t.Fatalf("Failed to get state after delete: %v", diags.Errors())
	}

	// Verify ALL fields match original state (resource still tracked)
	AssertStateFieldEquals(t, "CredentialId", stateAfterDelete.CredentialId, state.CredentialId)
	AssertStateFieldEquals(t, "Id", stateAfterDelete.Id, state.Id)
	AssertStateFieldEquals(t, "ProjectId", stateAfterDelete.ProjectId, state.ProjectId)
	AssertStateFieldEquals(t, "InstanceId", stateAfterDelete.InstanceId, state.InstanceId)
	AssertStateFieldEquals(t, "Username", stateAfterDelete.Username, state.Username)
	AssertStateFieldEquals(t, "Host", stateAfterDelete.Host, state.Host)

	t.Log("GOOD: State preserved when delete wait fails - user can retry")
}

func TestDelete_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "test-instance-456"
	credentialId := "test-credential-789"

	// Setup mock handler that returns error
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}/credentials/{credentialId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	}).Methods("DELETE")

	// Prepare request with current state
	schema := tc.GetSchema()
	state := CreateFullTestModel(projectId, instanceId, credentialId, "test-user", "test-host")
	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when API call fails")
	}

	t.Log("SUCCESS: Error handled correctly")
}

func TestDelete_ResourceAlreadyDeleted(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "test-instance-456"
	credentialId := "test-credential-789"

	// Setup mock handler that returns 404 Not Found
	tc.SetupDeleteCredentialHandler404()

	// Prepare request with current state
	schema := tc.GetSchema()
	state := CreateFullTestModel(projectId, instanceId, credentialId, "test-user", "test-host")
	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions - should succeed for idempotency
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed for idempotency when resource is 404, but got errors: %v", resp.Diagnostics.Errors())
	}

	t.Log("SUCCESS: Delete is idempotent - succeeds even when credential already deleted (404)")
}

func TestDelete_ResourceGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "test-instance-456"
	credentialId := "test-credential-789"

	// Setup mock handler that returns 410 Gone
	tc.SetupDeleteCredentialHandler410()

	// Prepare request with current state
	schema := tc.GetSchema()
	state := CreateFullTestModel(projectId, instanceId, credentialId, "test-user", "test-host")
	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions - should succeed for idempotency
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed for idempotency when resource is 410, but got errors: %v", resp.Diagnostics.Errors())
	}

	t.Log("SUCCESS: Delete is idempotent - succeeds even when credential is gone (410)")
}

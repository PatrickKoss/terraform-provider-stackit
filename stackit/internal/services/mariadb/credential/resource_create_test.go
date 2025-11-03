package mariadb

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestCreate_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "test-instance-456"
	credentialId := "test-credential-789"

	// Setup mock handlers
	createCalled := 0
	tc.SetupCreateCredentialHandler(credentialId, &createCalled)

	// Setup GetCredential to simulate slow response (triggers timeout)
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}/credentials/{credentialId}", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		jsonResp := fmt.Sprintf(`{
			"id": "%s",
			"uri": "mysql://user:pass@host:3306/db",
			"raw": {
				"credentials": {
					"host": "test-host",
					"password": "test-pass",
					"username": "test-user"
				}
			}
		}`, credentialId)
		w.Write([]byte(jsonResp))
	}).Methods("GET")

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, instanceId)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	// Assertions
	if createCalled == 0 {
		t.Fatal("CreateCredentials API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to context timeout")
	}

	// CRITICAL: Verify credential_id was saved
	var stateAfterCreate Model
	diags := resp.State.Get(tc.Ctx, &stateAfterCreate)
	if diags.HasError() {
		t.Logf("Warnings getting state: %v", diags)
	}

	// Verify idempotency fix
	if stateAfterCreate.CredentialId.IsNull() || stateAfterCreate.CredentialId.ValueString() == "" {
		t.Fatal("BUG: CredentialId should be saved to state immediately after API succeeds")
	}

	AssertStateFieldEquals(t, "CredentialId", stateAfterCreate.CredentialId, types.StringValue(credentialId))
	AssertStateFieldEquals(t, "Id", stateAfterCreate.Id, types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, instanceId, credentialId)))

	t.Log("GOOD: CredentialId saved even though wait failed - idempotency guaranteed")
}

func TestCreate_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "test-instance-456"
	credentialId := "test-credential-789"
	username := "test-user"
	host := "test-host.example.com"
	var port int64 = 3306

	// Setup mock handlers
	createCalled := 0
	tc.SetupCreateCredentialHandler(credentialId, &createCalled)
	tc.SetupGetCredentialHandler(credentialId, username, host, port)

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, instanceId)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	// Assertions
	if createCalled == 0 {
		t.Fatal("CreateCredentials API should have been called")
	}

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Extract final state
	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	// Verify all fields
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

	t.Log("SUCCESS: All state fields correctly populated")
}

func TestCreate_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "test-instance-456"

	// Setup mock handler that returns error
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}/credentials", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	}).Methods("POST")

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, instanceId)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	// Assertions
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when API call fails")
	}

	// Extract state
	var stateAfterCreate Model
	diags := resp.State.Get(tc.Ctx, &stateAfterCreate)
	if diags.HasError() {
		t.Logf("Warnings getting state: %v", diags)
	}

	// Verify CredentialId is null (nothing created)
	if !stateAfterCreate.CredentialId.IsNull() && stateAfterCreate.CredentialId.ValueString() != "" {
		t.Error("CredentialId should be null when API call fails")
	}

	t.Log("SUCCESS: Error handled correctly, no credential_id saved")
}

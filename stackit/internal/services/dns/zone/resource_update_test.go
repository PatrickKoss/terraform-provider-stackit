package dns

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUpdate_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	oldName := "my-old-zone"
	newName := "my-new-zone"
	dnsName := "example.com"

	// Setup mock handlers
	updateCalled := 0
	tc.SetupUpdateZoneHandler(&updateCalled)
	tc.SetupGetZoneHandler(zoneId, newName, dnsName, "UPDATE_SUCCEEDED")

	// Prepare request
	schema := tc.GetSchema()
	currentState := CreateTestModel(projectId, zoneId, oldName, dnsName)
	plan := CreateTestModel(projectId, zoneId, newName, dnsName)
	req := UpdateRequest(tc.Ctx, schema, plan, currentState)
	resp := UpdateResponse(tc.Ctx, schema, nil)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	if updateCalled == 0 {
		t.Fatal("PartialUpdateZone API should have been called")
	}

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Extract final state
	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	// Verify state updated with new values
	AssertStateFieldEquals(t, "Name", finalState.Name, types.StringValue(newName))
	AssertStateFieldEquals(t, "State", finalState.State, types.StringValue("UPDATE_SUCCEEDED"))

	t.Log("SUCCESS: State updated with new values")
}

func TestUpdate_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	oldName := "my-old-zone"
	newName := "my-new-zone"
	dnsName := "example.com"

	// Setup mock handlers
	updateCalled := 0
	tc.SetupUpdateZoneHandler(&updateCalled)

	// Setup GetZone to simulate slow response (triggers timeout)
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		jsonResp := fmt.Sprintf(`{
			"zone": {
				"id": "%s",
				"name": "%s",
				"dnsName": "%s",
				"state": "UPDATING",
				"acl": "0.0.0.0/0,::/0",
				"creationFinished": "2024-01-01T00:00:00Z",
				"creationStarted": "2024-01-01T00:00:00Z",
				"defaultTTL": 3600,
				"expireTime": 1209600,
				"negativeCache": 60,
				"primaryNameServer": "ns1.example.com",
				"refreshTime": 3600,
				"retryTime": 600,
				"serialNumber": 2024010100,
				"type": "primary",
				"updateFinished": "2024-01-01T00:00:00Z",
				"updateStarted": "2024-01-01T00:00:00Z",
				"visibility": "public"
			}
		}`, zoneId, newName, dnsName)
		w.Write([]byte(jsonResp))
	}).Methods("GET")

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Prepare request
	schema := tc.GetSchema()
	currentState := CreateTestModel(projectId, zoneId, oldName, dnsName)
	currentState.State = types.StringValue("UPDATE_SUCCEEDED")
	plan := CreateTestModel(projectId, zoneId, newName, dnsName)
	req := UpdateRequest(tc.Ctx, schema, plan, currentState)
	resp := UpdateResponse(tc.Ctx, schema, &currentState)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	if updateCalled == 0 {
		t.Fatal("PartialUpdateZone API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to context timeout")
	}

	// Verify state was NOT updated with new values
	var stateAfterUpdate Model
	diags := resp.State.Get(context.Background(), &stateAfterUpdate)
	if diags.HasError() {
		t.Fatalf("Failed to get state after update: %v", diags.Errors())
	}

	// Verify state has old values
	AssertStateFieldEquals(t, "Name", stateAfterUpdate.Name, types.StringValue(oldName))
	AssertStateFieldEquals(t, "State", stateAfterUpdate.State, types.StringValue("UPDATE_SUCCEEDED"))

	// Verify error message is helpful
	errorFound := false
	for _, diag := range resp.Diagnostics.Errors() {
		if diag.Summary() == "Error updating zone" {
			errorFound = true
			detail := diag.Detail()
			if detail == "" {
				t.Error("Error detail should not be empty")
			}
			t.Logf("Error message: %s", detail)
		}
	}
	if !errorFound {
		t.Error("Expected 'Error updating zone' diagnostic")
	}

	t.Log("GOOD: State preserved with old values when update wait fails")
}

func TestUpdate_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	oldName := "my-old-zone"
	newName := "my-new-zone"
	dnsName := "example.com"

	// Setup mock handler to return error
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "Internal server error"}`))
	}).Methods("PATCH")

	// Prepare request
	schema := tc.GetSchema()
	currentState := CreateTestModel(projectId, zoneId, oldName, dnsName)
	plan := CreateTestModel(projectId, zoneId, newName, dnsName)
	req := UpdateRequest(tc.Ctx, schema, plan, currentState)
	resp := UpdateResponse(tc.Ctx, schema, &currentState)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when API call fails")
	}

	t.Log("SUCCESS: Error returned when update API fails")
}

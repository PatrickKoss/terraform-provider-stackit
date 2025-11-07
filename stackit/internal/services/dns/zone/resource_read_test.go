package dns

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
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	zoneName := "my-test-zone"
	dnsName := "example.com"

	// Setup mock handler
	tc.SetupGetZoneHandler(zoneId, zoneName, dnsName, "CREATE_SUCCEEDED")

	// Prepare request
	schema := tc.GetSchema()
	state := CreateTestModel(projectId, zoneId, zoneName, dnsName)
	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(tc.Ctx, schema, &state)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Extract final state
	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	// Verify all fields
	AssertStateFieldEquals(t, "ZoneId", finalState.ZoneId, types.StringValue(zoneId))
	AssertStateFieldEquals(t, "Name", finalState.Name, types.StringValue(zoneName))
	AssertStateFieldEquals(t, "DnsName", finalState.DnsName, types.StringValue(dnsName))
	AssertStateFieldEquals(t, "State", finalState.State, types.StringValue("CREATE_SUCCEEDED"))

	t.Log("SUCCESS: All state fields correctly populated")
}

func TestRead_ResourceNotFound(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	zoneName := "my-test-zone"
	dnsName := "example.com"

	// Setup mock handler to return 404
	tc.SetupGetZoneHandlerWithStatus(http.StatusNotFound)

	// Prepare request
	schema := tc.GetSchema()
	state := CreateTestModel(projectId, zoneId, zoneName, dnsName)
	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(tc.Ctx, schema, &state)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions - should error (404 in Read is not automatically handled by the resource)
	// The error handling for 404 in Read depends on the specific implementation
	// In this case, the DNS zone resource logs an error for 404
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when zone is not found (404)")
	}

	t.Log("SUCCESS: Error returned when zone is 404")
}

func TestRead_ResourceGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	zoneName := "my-test-zone"
	dnsName := "example.com"

	// Setup mock handler to return zone with DELETE_SUCCEEDED state
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResp := fmt.Sprintf(`{
			"zone": {
				"id": "%s",
				"name": "%s",
				"dnsName": "%s",
				"state": "DELETE_SUCCEEDED",
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
		}`, zoneId, zoneName, dnsName)
		w.Write([]byte(jsonResp))
	}).Methods("GET")

	// Prepare request
	schema := tc.GetSchema()
	state := CreateTestModel(projectId, zoneId, zoneName, dnsName)
	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(tc.Ctx, schema, &state)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions - should not error, state should be removed
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should succeed when zone is DELETE_SUCCEEDED, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Verify state was removed (IsNull check on a required field)
	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	if diags.HasError() {
		t.Logf("State was removed (expected): %v", diags)
	}

	// Check if state is empty by checking a required field
	if !finalState.ZoneId.IsNull() && finalState.ZoneId.ValueString() != "" {
		t.Error("State should be removed when zone is DELETE_SUCCEEDED")
	}

	t.Log("SUCCESS: State removed when zone is DELETE_SUCCEEDED")
}

func TestRead_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	zoneName := "my-test-zone"
	dnsName := "example.com"

	// Setup mock handler to return error
	tc.SetupGetZoneHandlerWithStatus(http.StatusInternalServerError)

	// Prepare request
	schema := tc.GetSchema()
	state := CreateTestModel(projectId, zoneId, zoneName, dnsName)
	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(tc.Ctx, schema, &state)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when API call fails")
	}

	t.Log("SUCCESS: Error returned when read API fails")
}

func TestRead_DetectDrift(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	oldName := "my-old-zone"
	newName := "my-new-zone"
	dnsName := "example.com"

	// Setup mock handler with updated values
	tc.SetupGetZoneHandler(zoneId, newName, dnsName, "UPDATE_SUCCEEDED")

	// Prepare request with old state
	schema := tc.GetSchema()
	state := CreateTestModel(projectId, zoneId, oldName, dnsName)
	state.State = types.StringValue("CREATE_SUCCEEDED")
	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(tc.Ctx, schema, &state)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Extract final state
	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	// Verify state updated with new values (drift detected)
	AssertStateFieldEquals(t, "Name", finalState.Name, types.StringValue(newName))
	AssertStateFieldEquals(t, "State", finalState.State, types.StringValue("UPDATE_SUCCEEDED"))

	t.Log("SUCCESS: Drift detected and state updated with new values")
}

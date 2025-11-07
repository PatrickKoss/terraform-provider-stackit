package dns

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
	zoneId := "zone-abc-123"
	zoneName := "my-test-zone"
	dnsName := "example.com"

	// Setup mock handlers
	createCalled := 0
	tc.SetupCreateZoneHandler(zoneId, &createCalled)

	// Setup GetZone to simulate slow response (triggers timeout)
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		jsonResp := fmt.Sprintf(`{
			"zone": {
				"id": "%s",
				"name": "%s",
				"dnsName": "%s",
				"state": "CREATING",
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

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, "", zoneName, dnsName)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	// Assertions
	if createCalled == 0 {
		t.Fatal("CreateZone API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to context timeout")
	}

	// CRITICAL: Verify zone_id was saved
	var stateAfterCreate Model
	diags := resp.State.Get(context.Background(), &stateAfterCreate)
	if diags.HasError() {
		t.Logf("Warnings getting state: %v", diags)
	}

	// Verify idempotency fix
	if stateAfterCreate.ZoneId.IsNull() || stateAfterCreate.ZoneId.ValueString() == "" {
		t.Fatal("BUG: ZoneId should be saved to state immediately after API succeeds")
	}

	AssertStateFieldEquals(t, "ZoneId", stateAfterCreate.ZoneId, types.StringValue(zoneId))
	AssertStateFieldEquals(t, "Id", stateAfterCreate.Id, types.StringValue(fmt.Sprintf("%s,%s", projectId, zoneId)))

	t.Log("GOOD: ZoneId saved even though wait failed - idempotency guaranteed")
}

func TestCreate_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	zoneName := "my-test-zone"
	dnsName := "example.com"

	// Setup mock handlers
	createCalled := 0
	tc.SetupCreateZoneHandler(zoneId, &createCalled)
	tc.SetupGetZoneHandler(zoneId, zoneName, dnsName, "CREATE_SUCCEEDED")

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, "", zoneName, dnsName)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	// Assertions
	if createCalled == 0 {
		t.Fatal("CreateZone API should have been called")
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
	AssertStateFieldEquals(t, "ZoneId", finalState.ZoneId, types.StringValue(zoneId))
	AssertStateFieldEquals(t, "Name", finalState.Name, types.StringValue(zoneName))
	AssertStateFieldEquals(t, "DnsName", finalState.DnsName, types.StringValue(dnsName))
	AssertStateFieldEquals(t, "State", finalState.State, types.StringValue("CREATE_SUCCEEDED"))

	t.Log("SUCCESS: All state fields correctly populated")
}

func TestCreate_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneName := "my-test-zone"
	dnsName := "example.com"

	// Setup mock handler to return error
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "Internal server error"}`))
	}).Methods("POST")

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, "", zoneName, dnsName)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	// Assertions
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when API call fails")
	}

	// Verify ZoneId is null (nothing created)
	var stateAfterCreate Model
	diags := resp.State.Get(tc.Ctx, &stateAfterCreate)
	if diags.HasError() {
		t.Logf("Warnings getting state: %v", diags)
	}

	if !stateAfterCreate.ZoneId.IsNull() && stateAfterCreate.ZoneId.ValueString() != "" {
		t.Error("ZoneId should be null when API call fails")
	}

	t.Log("SUCCESS: ZoneId is null when create fails")
}

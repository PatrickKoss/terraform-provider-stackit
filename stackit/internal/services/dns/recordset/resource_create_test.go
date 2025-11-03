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
	recordSetId := "recordset-abc-123"
	name := "test.example.com"
	recordType := "A"
	records := []string{"192.168.1.1"}

	// Setup mock handlers
	createCalled := 0
	tc.SetupCreateRecordSetHandler(recordSetId, &createCalled)

	// Setup GetRecordSet to simulate slow response (triggers timeout)
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}/rrsets/{recordSetId}", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		jsonResp := fmt.Sprintf(`{
			"rrset": {
				"id": "%s",
				"name": "%s",
				"type": "%s",
				"ttl": 3600,
				"state": "CREATING",
				"active": true,
				"records": [{"content": "192.168.1.1"}]
			}
		}`, recordSetId, name, recordType)
		w.Write([]byte(jsonResp))
	}).Methods("GET")

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, zoneId, "", name, recordType, records)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	// Assertions
	if createCalled == 0 {
		t.Fatal("CreateRecordSet API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to context timeout")
	}

	// CRITICAL: Verify record_set_id was saved
	var stateAfterCreate Model
	diags := resp.State.Get(context.Background(), &stateAfterCreate)
	if diags.HasError() {
		t.Logf("Warnings getting state: %v", diags)
	}

	// Verify idempotency fix
	if stateAfterCreate.RecordSetId.IsNull() || stateAfterCreate.RecordSetId.ValueString() == "" {
		t.Fatal("BUG: RecordSetId should be saved to state immediately after API succeeds")
	}

	AssertStateFieldEquals(t, "RecordSetId", stateAfterCreate.RecordSetId, types.StringValue(recordSetId))
	AssertStateFieldEquals(t, "Id", stateAfterCreate.Id, types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, zoneId, recordSetId)))
	AssertStateFieldEquals(t, "ProjectId", stateAfterCreate.ProjectId, types.StringValue(projectId))
	AssertStateFieldEquals(t, "ZoneId", stateAfterCreate.ZoneId, types.StringValue(zoneId))

	t.Log("GOOD: RecordSetId saved even though wait failed - idempotency guaranteed")
}

func TestCreate_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-abc-123"
	name := "test.example.com"
	recordType := "A"
	records := []string{"192.168.1.1"}

	// Setup mock handlers
	createCalled := 0
	tc.SetupCreateRecordSetHandler(recordSetId, &createCalled)
	tc.SetupGetRecordSetHandler(RecordSetResponseData{
		RecordSetId: recordSetId,
		Name:        name,
		Records:     records,
		TTL:         3600,
		Type:        recordType,
		State:       "CREATE_SUCCEEDED",
		Comment:     "",
	})

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, zoneId, "", name, recordType, records)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	// Assertions
	if createCalled == 0 {
		t.Fatal("CreateRecordSet API should have been called")
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
	AssertStateFieldEquals(t, "RecordSetId", finalState.RecordSetId, types.StringValue(recordSetId))
	AssertStateFieldEquals(t, "Name", finalState.Name, types.StringValue(name))
	AssertStateFieldEquals(t, "Type", finalState.Type, types.StringValue(recordType))
	AssertStateFieldEquals(t, "State", finalState.State, types.StringValue("CREATE_SUCCEEDED"))
	AssertStateFieldInt64Equals(t, "TTL", finalState.TTL, types.Int64Value(3600))

	t.Log("SUCCESS: All state fields correctly populated")
}

func TestCreate_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	name := "test.example.com"
	recordType := "A"
	records := []string{"192.168.1.1"}

	// Setup mock handler to return error
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}/rrsets", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "Internal server error"}`))
	}).Methods("POST")

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, zoneId, "", name, recordType, records)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	// Assertions
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when API call fails")
	}

	// Verify RecordSetId is null (nothing created)
	var stateAfterCreate Model
	diags := resp.State.Get(tc.Ctx, &stateAfterCreate)
	if diags.HasError() {
		t.Logf("Warnings getting state: %v", diags)
	}

	if !stateAfterCreate.RecordSetId.IsNull() && stateAfterCreate.RecordSetId.ValueString() != "" {
		t.Error("RecordSetId should be null when API call fails")
	}

	t.Log("SUCCESS: RecordSetId is null when create fails")
}

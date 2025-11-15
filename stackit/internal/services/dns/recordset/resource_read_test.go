package dns

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
)

func TestRead_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-abc-123"
	name := "test.example.com"
	recordType := "A"
	records := []string{"192.168.1.1"}

	// Setup mock handler
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
	state := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, records)
	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(tc.Ctx, schema, &state)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions
	require.False(t, resp.Diagnostics.HasError(), "Expected no errors reading state")

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

	t.Log("SUCCESS: All state fields correctly populated")
}

func TestRead_ResourceNotFound(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-abc-123"
	name := "test.example.com"
	recordType := "A"
	records := []string{"192.168.1.1"}

	// Setup mock handler to return 404
	tc.SetupGetRecordSetHandlerWithStatus(http.StatusNotFound)

	// Prepare request
	schema := tc.GetSchema()
	state := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, records)
	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(tc.Ctx, schema, &state)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions - should error (404 in Read is not automatically handled by the resource)
	// The error handling for 404 in Read depends on the specific implementation
	// In this case, the DNS recordset resource logs an error for 404
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when record set is not found (404)")
	}

	t.Log("SUCCESS: Error returned when record set is 404")
}

func TestRead_ResourceGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-abc-123"
	name := "test.example.com"
	recordType := "A"
	records := []string{"192.168.1.1"}

	// Setup mock handler to return record set with DELETE_SUCCEEDED state
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}/rrsets/{recordSetId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResp := fmt.Sprintf(`{
			"rrset": {
				"id": "%s",
				"name": "%s",
				"type": "%s",
				"ttl": 3600,
				"state": "DELETE_SUCCEEDED",
				"active": true,
				"records": [{"content": "192.168.1.1"}]
			}
		}`, recordSetId, name, recordType)
		w.Write([]byte(jsonResp))
	}).Methods("GET")

	// Prepare request
	schema := tc.GetSchema()
	state := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, records)
	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(tc.Ctx, schema, &state)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions - should not error, state should be removed
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should succeed when record set is DELETE_SUCCEEDED, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Verify state was removed (IsNull check on a required field)
	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	if diags.HasError() {
		t.Logf("State was removed (expected): %v", diags)
	}

	// Check if state is empty by checking a required field
	if !finalState.RecordSetId.IsNull() && finalState.RecordSetId.ValueString() != "" {
		t.Error("State should be removed when record set is DELETE_SUCCEEDED")
	}

	t.Log("SUCCESS: State removed when record set is DELETE_SUCCEEDED")
}

func TestRead_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-abc-123"
	name := "test.example.com"
	recordType := "A"
	records := []string{"192.168.1.1"}

	// Setup mock handler to return error
	tc.SetupGetRecordSetHandlerWithStatus(http.StatusInternalServerError)

	// Prepare request
	schema := tc.GetSchema()
	state := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, records)
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
	recordSetId := "recordset-abc-123"
	name := "test.example.com"
	recordType := "A"
	oldRecords := []string{"192.168.1.1"}
	newRecords := []string{"192.168.1.2"}

	// Setup mock handler with updated values
	tc.SetupGetRecordSetHandler(RecordSetResponseData{
		RecordSetId: recordSetId,
		Name:        name,
		Records:     newRecords,
		TTL:         3600,
		Type:        recordType,
		State:       "UPDATE_SUCCEEDED",
		Comment:     "",
	})

	// Prepare request with old state
	schema := tc.GetSchema()
	state := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, oldRecords)
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
	AssertStateFieldEquals(t, "State", finalState.State, types.StringValue("UPDATE_SUCCEEDED"))

	// Verify records were updated
	var recordsList []string
	finalState.Records.ElementsAs(tc.Ctx, &recordsList, false)
	if len(recordsList) != 1 || recordsList[0] != "192.168.1.2" {
		t.Errorf("Records should have new values: got %v, want %v", recordsList, newRecords)
	}

	t.Log("SUCCESS: Drift detected and state updated with new values")
}

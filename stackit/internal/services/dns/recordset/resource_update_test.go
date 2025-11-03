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
	recordSetId := "recordset-abc-123"
	name := "test.example.com"
	recordType := "A"
	oldRecords := []string{"192.168.1.1"}
	newRecords := []string{"192.168.1.2"}

	// Setup mock handlers
	updateCalled := 0
	tc.SetupUpdateRecordSetHandler(&updateCalled)
	tc.SetupGetRecordSetHandler(RecordSetResponseData{
		RecordSetId: recordSetId,
		Name:        name,
		Records:     newRecords,
		TTL:         3600,
		Type:        recordType,
		State:       "UPDATE_SUCCEEDED",
		Comment:     "",
	})

	// Prepare request
	schema := tc.GetSchema()
	currentState := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, oldRecords)
	plan := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, newRecords)
	req := UpdateRequest(tc.Ctx, schema, plan, currentState)
	resp := UpdateResponse(tc.Ctx, schema, nil)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	if updateCalled == 0 {
		t.Fatal("PartialUpdateRecordSet API should have been called")
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
	AssertStateFieldEquals(t, "State", finalState.State, types.StringValue("UPDATE_SUCCEEDED"))

	// Verify records were updated
	var newRecordsList []string
	finalState.Records.ElementsAs(tc.Ctx, &newRecordsList, false)
	if len(newRecordsList) != 1 || newRecordsList[0] != "192.168.1.2" {
		t.Errorf("Records not updated correctly: got %v, want %v", newRecordsList, newRecords)
	}

	t.Log("SUCCESS: State updated with new values")
}

func TestUpdate_ContextCanceledDuringWait(t *testing.T) {
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

	// Setup mock handlers
	updateCalled := 0
	tc.SetupUpdateRecordSetHandler(&updateCalled)

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
				"state": "UPDATING",
				"active": true,
				"records": [{"content": "192.168.1.2"}]
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
	currentState := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, oldRecords)
	currentState.State = types.StringValue("UPDATE_SUCCEEDED")
	plan := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, newRecords)
	req := UpdateRequest(tc.Ctx, schema, plan, currentState)
	resp := UpdateResponse(tc.Ctx, schema, &currentState)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	if updateCalled == 0 {
		t.Fatal("PartialUpdateRecordSet API should have been called")
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

	// Verify state has old values (not new planned values)
	AssertStateFieldEquals(t, "State", stateAfterUpdate.State, types.StringValue("UPDATE_SUCCEEDED"))

	// Verify old records are preserved
	var recordsList []string
	stateAfterUpdate.Records.ElementsAs(context.Background(), &recordsList, false)
	if len(recordsList) != 1 || recordsList[0] != "192.168.1.1" {
		t.Errorf("Records should have old values: got %v, want %v", recordsList, oldRecords)
	}

	// Verify error message is helpful
	errorFound := false
	for _, diag := range resp.Diagnostics.Errors() {
		if diag.Summary() == "Error updating record set" {
			errorFound = true
			detail := diag.Detail()
			if detail == "" {
				t.Error("Error detail should not be empty")
			}
			t.Logf("Error message: %s", detail)
		}
	}
	if !errorFound {
		t.Error("Expected 'Error updating record set' diagnostic")
	}

	t.Log("GOOD: State preserved with old values when update wait fails")
}

func TestUpdate_APICallFails(t *testing.T) {
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

	// Setup mock handler to return error
	tc.Router.HandleFunc("/v1/projects/{projectId}/zones/{zoneId}/rrsets/{recordSetId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "Internal server error"}`))
	}).Methods("PATCH")

	// Prepare request
	schema := tc.GetSchema()
	currentState := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, oldRecords)
	plan := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, newRecords)
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

package dns

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
	zoneId := "zone-abc-123"
	recordSetId := "recordset-abc-123"
	name := "test.example.com"
	recordType := "A"
	records := []string{"192.168.1.1"}

	// Setup mock handlers
	deleteCalled := 0
	tc.SetupDeleteRecordSetHandler(&deleteCalled)
	// Mock GetRecordSet to return record set with DELETE_SUCCEEDED state
	tc.SetupGetRecordSetHandler(RecordSetResponseData{
		RecordSetId: recordSetId,
		Name:        name,
		Records:     records,
		TTL:         3600,
		Type:        recordType,
		State:       "DELETE_SUCCEEDED",
		Comment:     "",
	})

	// Prepare request
	schema := tc.GetSchema()
	state := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, records)
	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if deleteCalled == 0 {
		t.Fatal("DeleteRecordSet API should have been called")
	}

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	t.Log("SUCCESS: Record set deleted successfully")
}

func TestDelete_ContextCanceledDuringWait(t *testing.T) {
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
	deleteCalled := 0
	tc.SetupDeleteRecordSetHandler(&deleteCalled)

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
				"state": "DELETING",
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
	state := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, records)
	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, &state)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if deleteCalled == 0 {
		t.Fatal("DeleteRecordSet API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to context timeout")
	}

	// Verify state was NOT removed
	var stateAfterDelete Model
	diags := resp.State.Get(context.Background(), &stateAfterDelete)
	if diags.HasError() {
		t.Fatalf("Failed to get state after delete: %v", diags.Errors())
	}

	// Verify all fields match original state (resource still tracked)
	AssertStateFieldEquals(t, "RecordSetId", stateAfterDelete.RecordSetId, state.RecordSetId)
	AssertStateFieldEquals(t, "Id", stateAfterDelete.Id, state.Id)
	AssertStateFieldEquals(t, "ProjectId", stateAfterDelete.ProjectId, state.ProjectId)
	AssertStateFieldEquals(t, "ZoneId", stateAfterDelete.ZoneId, state.ZoneId)
	AssertStateFieldEquals(t, "Name", stateAfterDelete.Name, state.Name)
	AssertStateFieldEquals(t, "Type", stateAfterDelete.Type, state.Type)

	// Verify error message is helpful
	errorFound := false
	for _, diag := range resp.Diagnostics.Errors() {
		if diag.Summary() == "Error deleting record set" {
			errorFound = true
			detail := diag.Detail()
			if detail == "" {
				t.Error("Error detail should not be empty")
			}
			t.Logf("Error message: %s", detail)
		}
	}
	if !errorFound {
		t.Error("Expected 'Error deleting record set' diagnostic")
	}

	t.Log("GOOD: State preserved when delete wait fails")
}

func TestDelete_APICallFails(t *testing.T) {
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
	tc.SetupDeleteRecordSetHandlerWithStatus(http.StatusInternalServerError, nil)

	// Prepare request
	schema := tc.GetSchema()
	state := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, records)
	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, &state)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when API call fails")
	}

	t.Log("SUCCESS: Error returned when delete API fails")
}

func TestDelete_ResourceAlreadyDeleted(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-abc-123"
	name := "test.example.com"
	recordType := "A"
	records := []string{"192.168.1.1"}

	// Setup mock handler to return 404 Not Found (idempotency test)
	deleteCalled := 0
	tc.SetupDeleteRecordSetHandlerWithStatus(http.StatusNotFound, &deleteCalled)

	// Prepare request
	schema := tc.GetSchema()
	state := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, records)
	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if deleteCalled == 0 {
		t.Fatal("DeleteRecordSet API should have been called")
	}

	// CRITICAL: Should NOT error (idempotency)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed for idempotency when resource is 404, but got errors: %v", resp.Diagnostics.Errors())
	}

	t.Log("SUCCESS: Delete is idempotent - 404 treated as success")
}

func TestDelete_ResourceGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-abc-123"
	name := "test.example.com"
	recordType := "A"
	records := []string{"192.168.1.1"}

	// Setup mock handler to return 410 Gone (idempotency test)
	deleteCalled := 0
	tc.SetupDeleteRecordSetHandlerWithStatus(http.StatusGone, &deleteCalled)

	// Prepare request
	schema := tc.GetSchema()
	state := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, records)
	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if deleteCalled == 0 {
		t.Fatal("DeleteRecordSet API should have been called")
	}

	// CRITICAL: Should NOT error (idempotency)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed for idempotency when resource is 410, but got errors: %v", resp.Diagnostics.Errors())
	}

	t.Log("SUCCESS: Delete is idempotent - 410 treated as success")
}

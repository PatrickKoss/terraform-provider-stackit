package cdn

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestDelete_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	distributionId := "dist-abc-123"
	customDomainName := "example.com"

	// Setup mock handlers
	deleteCalled := 0
	tc.SetupDeleteCustomDomainHandler(&deleteCalled)
	tc.SetupGetCustomDomainHandlerGone() // Returns 410 Gone after deletion

	// Prepare request
	schema := tc.GetSchema()
	model := BuildTestModel(projectId, distributionId, customDomainName)
	model.ID = types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, distributionId, customDomainName))

	req := DeleteRequest(tc.Ctx, schema, model)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if deleteCalled == 0 {
		t.Fatal("DeleteCustomDomain API should have been called")
	}

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	t.Log("SUCCESS: Delete completed successfully")
}

func TestDelete_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	distributionId := "dist-abc-123"
	customDomainName := "example.com"

	// Current state (before delete)
	state := CustomDomainModel{
		ProjectId:      types.StringValue(projectId),
		DistributionId: types.StringValue(distributionId),
		Name:           types.StringValue(customDomainName),
		ID:             types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, distributionId, customDomainName)),
		Status:         types.StringValue("ACTIVE"),
		Errors:         types.ListNull(types.StringType),
		Certificate:    types.ObjectNull(certificateTypes),
	}

	// Setup mock handlers
	deleteCalled := 0
	tc.SetupDeleteCustomDomainHandler(&deleteCalled)

	// Setup GetCustomDomain to simulate slow response (triggers timeout)
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}/customDomains/{name}", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		jsonResp := fmt.Sprintf(`{
			"customDomain": {
				"name": "%s",
				"status": "DELETING"
			},
			"certificate": {
				"type": "managed"
			}
		}`, customDomainName)
		w.Write([]byte(jsonResp))
	}).Methods("GET")

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Prepare request
	schema := tc.GetSchema()
	req := DeleteRequest(tc.Ctx, schema, state)
	// IMPORTANT: Initialize response with current state (simulates framework behavior)
	resp := DeleteResponse(tc.Ctx, schema, &state)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if deleteCalled == 0 {
		t.Fatal("DeleteCustomDomain API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to context timeout")
	}

	// Verify helpful error message
	errMsgs := resp.Diagnostics.Errors()
	foundHelpfulMsg := false
	for _, errMsg := range errMsgs {
		detail := errMsg.Detail()
		if containsString(detail, "STACKIT Portal") || containsString(detail, "retry") {
			foundHelpfulMsg = true
			break
		}
	}
	if !foundHelpfulMsg {
		t.Error("Error message should guide user to check Portal or retry")
	}

	// After timeout, verify state was NOT removed (resource still tracked)
	var stateAfterDelete CustomDomainModel
	diags := resp.State.Get(context.Background(), &stateAfterDelete)
	if diags.HasError() {
		t.Fatalf("Failed to get state after delete: %v", diags.Errors())
	}

	// Verify all fields match original state
	AssertStateFieldEquals(t, "ID", stateAfterDelete.ID, state.ID)
	AssertStateFieldEquals(t, "ProjectId", stateAfterDelete.ProjectId, state.ProjectId)
	AssertStateFieldEquals(t, "DistributionId", stateAfterDelete.DistributionId, state.DistributionId)
	AssertStateFieldEquals(t, "Name", stateAfterDelete.Name, state.Name)
	AssertStateFieldEquals(t, "Status", stateAfterDelete.Status, state.Status)

	t.Log("CORRECT: State preserved when delete wait failed")
}

func TestDelete_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	distributionId := "dist-abc-123"
	customDomainName := "example.com"

	// Setup mock handlers - DeleteCustomDomain returns 500 error
	deleteCalled := 0
	tc.SetupDeleteCustomDomainHandlerError(&deleteCalled)

	// Prepare request
	schema := tc.GetSchema()
	model := BuildTestModel(projectId, distributionId, customDomainName)
	model.ID = types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, distributionId, customDomainName))

	req := DeleteRequest(tc.Ctx, schema, model)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if deleteCalled == 0 {
		t.Fatal("DeleteCustomDomain API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from API call")
	}

	t.Log("CORRECT: Error returned when API call fails")
}

func TestDelete_ResourceAlreadyDeleted(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	distributionId := "dist-abc-123"
	customDomainName := "example.com"

	// Setup mock handlers - DeleteCustomDomain returns 404 Not Found
	deleteCalled := 0
	tc.SetupDeleteCustomDomainHandlerWithStatus(http.StatusNotFound, &deleteCalled)

	// Prepare request
	schema := tc.GetSchema()
	model := BuildTestModel(projectId, distributionId, customDomainName)
	model.ID = types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, distributionId, customDomainName))

	req := DeleteRequest(tc.Ctx, schema, model)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if deleteCalled == 0 {
		t.Fatal("DeleteCustomDomain API should have been called")
	}

	// CRITICAL: Should NOT error (idempotency)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed for idempotency when resource is 404, but got errors: %v", resp.Diagnostics.Errors())
	}

	t.Log("SUCCESS: Delete is idempotent (handles 404 gracefully)")
}

func TestDelete_ResourceGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	distributionId := "dist-abc-123"
	customDomainName := "example.com"

	// Setup mock handlers - DeleteCustomDomain returns 410 Gone
	deleteCalled := 0
	tc.SetupDeleteCustomDomainHandlerWithStatus(http.StatusGone, &deleteCalled)

	// Prepare request
	schema := tc.GetSchema()
	model := BuildTestModel(projectId, distributionId, customDomainName)
	model.ID = types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, distributionId, customDomainName))

	req := DeleteRequest(tc.Ctx, schema, model)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if deleteCalled == 0 {
		t.Fatal("DeleteCustomDomain API should have been called")
	}

	// CRITICAL: Should NOT error (idempotency)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed for idempotency when resource is 410, but got errors: %v", resp.Diagnostics.Errors())
	}

	t.Log("SUCCESS: Delete is idempotent (handles 410 gracefully)")
}

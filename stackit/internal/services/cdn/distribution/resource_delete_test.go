package cdn

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestDelete_Success tests that a successful distribution deletion works correctly
func TestDelete_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	distributionId := "distribution-abc-123"

	deleteCalled := 0
	tc.SetupDeleteDistributionHandler(&deleteCalled)

	// Setup GetDistribution to return 410 Gone (resource deleted)
	tc.SetupGetDistributionHandlerWithStatus(http.StatusGone)

	schema := tc.GetSchema()

	backendObj, _ := types.ObjectValue(backendTypes, map[string]attr.Value{
		"type":                   types.StringValue("http"),
		"origin_url":             types.StringValue("https://example.com"),
		"origin_request_headers": types.MapNull(types.StringType),
		"geofencing":             types.MapNull(geofencingTypes.ElemType),
	})

	regionsList, _ := types.ListValue(types.StringType, []attr.Value{
		types.StringValue("EU"),
	})

	configObj, _ := types.ObjectValue(configTypes, map[string]attr.Value{
		"backend":           backendObj,
		"regions":           regionsList,
		"blocked_countries": types.ListNull(types.StringType),
		"optimizer":         types.ObjectNull(optimizerTypes),
	})

	model := CreateTestModel(projectId, distributionId, configObj)
	req := DeleteRequest(tc.Ctx, schema, model)
	resp := DeleteResponse(tc.Ctx, schema, &model)

	tc.Resource.Delete(tc.Ctx, req, resp)

	if deleteCalled == 0 {
		t.Fatal("DeleteDistribution API should have been called")
	}

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	t.Log("SUCCESS: Distribution deleted successfully")
}

// TestDelete_ContextCanceledDuringWait tests behavior when delete wait times out
// and state is not incorrectly removed
func TestDelete_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	distributionId := "distribution-abc-123"

	deleteCalled := 0
	tc.SetupDeleteDistributionHandler(&deleteCalled)

	// Setup GetDistribution to simulate slow response (triggers timeout)
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"distribution":{"id":"dist123","status":"DELETING"}}`))
	}).Methods("GET")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	schema := tc.GetSchema()

	backendObj, _ := types.ObjectValue(backendTypes, map[string]attr.Value{
		"type":                   types.StringValue("http"),
		"origin_url":             types.StringValue("https://example.com"),
		"origin_request_headers": types.MapNull(types.StringType),
		"geofencing":             types.MapNull(geofencingTypes.ElemType),
	})

	regionsList, _ := types.ListValue(types.StringType, []attr.Value{
		types.StringValue("EU"),
	})

	configObj, _ := types.ObjectValue(configTypes, map[string]attr.Value{
		"backend":           backendObj,
		"regions":           regionsList,
		"blocked_countries": types.ListNull(types.StringType),
		"optimizer":         types.ObjectNull(optimizerTypes),
	})

	model := CreateTestModel(projectId, distributionId, configObj)
	req := DeleteRequest(tc.Ctx, schema, model)
	resp := DeleteResponse(tc.Ctx, schema, &model)

	tc.Resource.Delete(tc.Ctx, req, resp)

	if deleteCalled == 0 {
		t.Fatal("DeleteDistribution API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to context timeout")
	}

	// Verify helpful error message
	errMsgs := resp.Diagnostics.Errors()
	if len(errMsgs) == 0 {
		t.Fatal("Expected error message in diagnostics")
	}
	errStr := errMsgs[0].Summary()
	if errStr != "Error deleting CDN distribution" {
		t.Errorf("Expected helpful error message, got: %s", errStr)
	}

	// After timeout, verify state was NOT removed
	var stateAfterDelete Model
	diags := resp.State.Get(tc.Ctx, &stateAfterDelete)
	if diags.HasError() {
		t.Fatalf("Failed to get state after delete: %v", diags.Errors())
	}

	// Verify ALL fields match original state (resource still tracked)
	AssertStateFieldEquals(t, "DistributionId", stateAfterDelete.DistributionId, model.DistributionId)
	AssertStateFieldEquals(t, "ID", stateAfterDelete.ID, model.ID)
	AssertStateFieldEquals(t, "ProjectId", stateAfterDelete.ProjectId, model.ProjectId)

	t.Log("PASS: State NOT removed when delete wait fails - resource still tracked")
}

// TestDelete_APICallFails tests that when the Delete API call fails, an error is returned
func TestDelete_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	distributionId := "distribution-abc-123"

	// Setup DeleteDistribution to return an error
	deleteCalled := 0
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}", func(w http.ResponseWriter, r *http.Request) {
		deleteCalled++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}).Methods("DELETE")

	schema := tc.GetSchema()

	backendObj, _ := types.ObjectValue(backendTypes, map[string]attr.Value{
		"type":                   types.StringValue("http"),
		"origin_url":             types.StringValue("https://example.com"),
		"origin_request_headers": types.MapNull(types.StringType),
		"geofencing":             types.MapNull(geofencingTypes.ElemType),
	})

	regionsList, _ := types.ListValue(types.StringType, []attr.Value{
		types.StringValue("EU"),
	})

	configObj, _ := types.ObjectValue(configTypes, map[string]attr.Value{
		"backend":           backendObj,
		"regions":           regionsList,
		"blocked_countries": types.ListNull(types.StringType),
		"optimizer":         types.ObjectNull(optimizerTypes),
	})

	model := CreateTestModel(projectId, distributionId, configObj)
	req := DeleteRequest(tc.Ctx, schema, model)
	resp := DeleteResponse(tc.Ctx, schema, &model)

	tc.Resource.Delete(tc.Ctx, req, resp)

	if deleteCalled == 0 {
		t.Fatal("DeleteDistribution API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to API failure")
	}

	t.Log("PASS: Error returned when API call fails")
}

// TestDelete_ResourceAlreadyDeleted tests that delete is idempotent (404 treated as success)
func TestDelete_ResourceAlreadyDeleted(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	distributionId := "distribution-abc-123"

	deleteCalled := 0
	tc.SetupDeleteDistributionHandlerWithStatus(http.StatusNotFound, &deleteCalled)

	schema := tc.GetSchema()

	backendObj, _ := types.ObjectValue(backendTypes, map[string]attr.Value{
		"type":                   types.StringValue("http"),
		"origin_url":             types.StringValue("https://example.com"),
		"origin_request_headers": types.MapNull(types.StringType),
		"geofencing":             types.MapNull(geofencingTypes.ElemType),
	})

	regionsList, _ := types.ListValue(types.StringType, []attr.Value{
		types.StringValue("EU"),
	})

	configObj, _ := types.ObjectValue(configTypes, map[string]attr.Value{
		"backend":           backendObj,
		"regions":           regionsList,
		"blocked_countries": types.ListNull(types.StringType),
		"optimizer":         types.ObjectNull(optimizerTypes),
	})

	model := CreateTestModel(projectId, distributionId, configObj)
	req := DeleteRequest(tc.Ctx, schema, model)
	resp := DeleteResponse(tc.Ctx, schema, &model)

	tc.Resource.Delete(tc.Ctx, req, resp)

	if deleteCalled == 0 {
		t.Fatal("DeleteDistribution API should have been called")
	}

	// Should succeed for idempotency
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed for idempotency (404), but got errors: %v", resp.Diagnostics.Errors())
	}

	t.Log("PASS: Delete is idempotent - 404 treated as success")
}

// TestDelete_ResourceGone tests that delete treats 410 Gone as success
func TestDelete_ResourceGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	distributionId := "distribution-abc-123"

	deleteCalled := 0
	tc.SetupDeleteDistributionHandlerWithStatus(http.StatusGone, &deleteCalled)

	schema := tc.GetSchema()

	backendObj, _ := types.ObjectValue(backendTypes, map[string]attr.Value{
		"type":                   types.StringValue("http"),
		"origin_url":             types.StringValue("https://example.com"),
		"origin_request_headers": types.MapNull(types.StringType),
		"geofencing":             types.MapNull(geofencingTypes.ElemType),
	})

	regionsList, _ := types.ListValue(types.StringType, []attr.Value{
		types.StringValue("EU"),
	})

	configObj, _ := types.ObjectValue(configTypes, map[string]attr.Value{
		"backend":           backendObj,
		"regions":           regionsList,
		"blocked_countries": types.ListNull(types.StringType),
		"optimizer":         types.ObjectNull(optimizerTypes),
	})

	model := CreateTestModel(projectId, distributionId, configObj)
	req := DeleteRequest(tc.Ctx, schema, model)
	resp := DeleteResponse(tc.Ctx, schema, &model)

	tc.Resource.Delete(tc.Ctx, req, resp)

	if deleteCalled == 0 {
		t.Fatal("DeleteDistribution API should have been called")
	}

	// Should succeed for idempotency
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed for idempotency (410), but got errors: %v", resp.Diagnostics.Errors())
	}

	t.Log("PASS: Delete is idempotent - 410 treated as success")
}

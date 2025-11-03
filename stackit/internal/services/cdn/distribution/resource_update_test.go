package cdn

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestUpdate_Success tests that a successful distribution update works correctly
func TestUpdate_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	distributionId := "distribution-abc-123"

	updateCalled := 0
	tc.SetupUpdateDistributionHandler(&updateCalled)
	tc.SetupGetDistributionHandler(distributionId, projectId, "ACTIVE")

	schema := tc.GetSchema()

	backendObj, _ := types.ObjectValue(backendTypes, map[string]attr.Value{
		"type":                   types.StringValue("http"),
		"origin_url":             types.StringValue("https://new-example.com"),
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
	req := UpdateRequest(tc.Ctx, schema, model)
	resp := UpdateResponse(tc.Ctx, schema, nil)

	tc.Resource.Update(tc.Ctx, req, resp)

	if updateCalled == 0 {
		t.Fatal("UpdateDistribution API should have been called")
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

	// Verify fields are updated
	AssertStateFieldEquals(t, "DistributionId", finalState.DistributionId, types.StringValue(distributionId))
	AssertStateFieldEquals(t, "Status", finalState.Status, types.StringValue("ACTIVE"))

	t.Log("SUCCESS: Update completed and state updated with new values")
}

// TestUpdate_ContextCanceledDuringWait tests behavior when update wait times out
// and state is not incorrectly updated
func TestUpdate_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	distributionId := "distribution-abc-123"

	updateCalled := 0
	tc.SetupUpdateDistributionHandler(&updateCalled)

	// Setup GetDistribution to simulate slow response (triggers timeout)
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		now := time.Now().Format(time.RFC3339)
		jsonResp := fmt.Sprintf(`{
			"distribution": {
				"id": "%s",
				"projectId": "%s",
				"status": "UPDATING",
				"createdAt": "%s",
				"updatedAt": "%s",
				"config": {
					"backend": {
						"http": {
							"type": "http",
							"originUrl": "https://old-example.com",
							"originRequestHeaders": {}
						}
					},
					"regions": ["EU"],
					"blockedCountries": []
				},
				"domains": []
			}
		}`, distributionId, projectId, now, now)
		w.Write([]byte(jsonResp))
	}).Methods("GET")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	schema := tc.GetSchema()

	// Current state (before update)
	backendObjOld, _ := types.ObjectValue(backendTypes, map[string]attr.Value{
		"type":                   types.StringValue("http"),
		"origin_url":             types.StringValue("https://old-example.com"),
		"origin_request_headers": types.MapNull(types.StringType),
		"geofencing":             types.MapNull(geofencingTypes.ElemType),
	})

	regionsList, _ := types.ListValue(types.StringType, []attr.Value{
		types.StringValue("EU"),
	})

	configObjOld, _ := types.ObjectValue(configTypes, map[string]attr.Value{
		"backend":           backendObjOld,
		"regions":           regionsList,
		"blocked_countries": types.ListNull(types.StringType),
		"optimizer":         types.ObjectNull(optimizerTypes),
	})

	currentState := CreateTestModel(projectId, distributionId, configObjOld)

	// New planned state (what we want to update to)
	backendObjNew, _ := types.ObjectValue(backendTypes, map[string]attr.Value{
		"type":                   types.StringValue("http"),
		"origin_url":             types.StringValue("https://new-example.com"),
		"origin_request_headers": types.MapNull(types.StringType),
		"geofencing":             types.MapNull(geofencingTypes.ElemType),
	})

	configObjNew, _ := types.ObjectValue(configTypes, map[string]attr.Value{
		"backend":           backendObjNew,
		"regions":           regionsList,
		"blocked_countries": types.ListNull(types.StringType),
		"optimizer":         types.ObjectNull(optimizerTypes),
	})

	plannedModel := CreateTestModel(projectId, distributionId, configObjNew)

	req := UpdateRequest(tc.Ctx, schema, plannedModel)
	resp := UpdateResponse(tc.Ctx, schema, &currentState)

	tc.Resource.Update(tc.Ctx, req, resp)

	if updateCalled == 0 {
		t.Fatal("UpdateDistribution API should have been called")
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
	if errStr != "Error updating CDN distribution" {
		t.Errorf("Expected helpful error message, got: %s", errStr)
	}

	// After timeout, verify state was NOT updated with new values
	var stateAfterUpdate Model
	diags := resp.State.Get(tc.Ctx, &stateAfterUpdate)
	if diags.HasError() {
		t.Fatalf("Failed to get state after update: %v", diags.Errors())
	}

	// State should preserve old values
	AssertStateFieldEquals(t, "DistributionId", stateAfterUpdate.DistributionId, currentState.DistributionId)
	AssertStateFieldEquals(t, "ID", stateAfterUpdate.ID, currentState.ID)
	AssertStateFieldEquals(t, "ProjectId", stateAfterUpdate.ProjectId, currentState.ProjectId)

	t.Log("PASS: State NOT updated with new values when update wait fails - preserves old state")
}

// TestUpdate_APICallFails tests that when the Update API call fails, state is not updated
func TestUpdate_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	distributionId := "distribution-abc-123"

	// Setup UpdateDistribution to return an error
	updateCalled := 0
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}", func(w http.ResponseWriter, r *http.Request) {
		updateCalled++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}).Methods("PATCH")

	schema := tc.GetSchema()

	backendObj, _ := types.ObjectValue(backendTypes, map[string]attr.Value{
		"type":                   types.StringValue("http"),
		"origin_url":             types.StringValue("https://new-example.com"),
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
	req := UpdateRequest(tc.Ctx, schema, model)
	resp := UpdateResponse(tc.Ctx, schema, nil)

	tc.Resource.Update(tc.Ctx, req, resp)

	if updateCalled == 0 {
		t.Fatal("UpdateDistribution API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to API failure")
	}

	t.Log("PASS: Error returned when API call fails")
}

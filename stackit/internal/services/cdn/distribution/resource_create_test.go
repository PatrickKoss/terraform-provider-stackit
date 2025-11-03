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

// TestCreate_ContextCanceledDuringWait tests that the distribution ID is saved to state
// immediately after API call succeeds, preventing orphaned resources
func TestCreate_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	distributionId := "distribution-abc-123"

	createCalled := 0
	tc.SetupCreateDistributionHandler(distributionId, &createCalled)

	// Setup GetDistribution to simulate slow response (triggers timeout)
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"distribution":{"id":"dist123","status":"CREATING"}}`))
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

	model := CreateTestModel(projectId, "", configObj)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	if createCalled == 0 {
		t.Fatal("CreateDistribution API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to context timeout")
	}

	var stateAfterCreate Model
	resp.State.Get(tc.Ctx, &stateAfterCreate)

	// CRITICAL: Verify distribution_id was saved despite wait timeout
	if stateAfterCreate.DistributionId.IsNull() || stateAfterCreate.DistributionId.ValueString() == "" {
		t.Fatal("IDEMPOTENCY BUG: DistributionId should be saved to state immediately after API succeeds")
	}

	AssertStateFieldEquals(t, "DistributionId", stateAfterCreate.DistributionId, types.StringValue(distributionId))
	AssertStateFieldEquals(t, "ID", stateAfterCreate.ID, types.StringValue(fmt.Sprintf("%s,%s", projectId, distributionId)))

	t.Log("PASS: DistributionId saved even though wait failed - idempotency guaranteed")
}

// TestCreate_Success tests that a successful distribution creation works correctly
func TestCreate_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	distributionId := "distribution-abc-123"

	createCalled := 0
	tc.SetupCreateDistributionHandler(distributionId, &createCalled)
	tc.SetupGetDistributionHandler(distributionId, projectId, "ACTIVE")

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

	model := CreateTestModel(projectId, "", configObj)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	if createCalled == 0 {
		t.Fatal("CreateDistribution API should have been called")
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
	AssertStateFieldEquals(t, "DistributionId", finalState.DistributionId, types.StringValue(distributionId))
	AssertStateFieldEquals(t, "Status", finalState.Status, types.StringValue("ACTIVE"))

	t.Log("SUCCESS: All state fields correctly populated")
}

// TestCreate_APICallFails tests that when the Create API call fails, no state is saved
func TestCreate_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"

	// Setup CreateDistribution to return an error
	createCalled := 0
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions", func(w http.ResponseWriter, r *http.Request) {
		createCalled++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}).Methods("POST")

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

	model := CreateTestModel(projectId, "", configObj)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	if createCalled == 0 {
		t.Fatal("CreateDistribution API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to API failure")
	}

	// Verify ResourceId is null (nothing created)
	var stateAfterCreate Model
	resp.State.Get(tc.Ctx, &stateAfterCreate)

	if !stateAfterCreate.DistributionId.IsNull() {
		t.Fatal("DistributionId should be null when API call fails")
	}

	t.Log("PASS: No state saved when API call fails")
}

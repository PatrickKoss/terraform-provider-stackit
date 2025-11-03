package mariadb

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
	instanceId := "instance-abc-123"
	instanceName := "my-test-instance"
	planId := "plan-123"
	planName := "stackit-mariadb-1.4.10-single"
	version := "1.4.10"

	createCalled := 0
	tc.SetupCreateInstanceHandler(instanceId, &createCalled)
	tc.SetupListOfferingsHandler(version, planId, planName)

	// Setup GetInstance to simulate slow response (triggers timeout)
	getInstanceCalled := 0
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		getInstanceCalled++
		time.Sleep(150 * time.Millisecond) // Longer than context timeout
		w.Header().Set("Content-Type", "application/json")
		jsonResp := fmt.Sprintf(`{
			"instanceId": "%s",
			"name": "%s",
			"planId": "%s",
			"status": "creating",
			"cfGuid": "cf-guid",
			"cfSpaceGuid": "cf-space-guid",
			"cfOrganizationGuid": "cf-org-guid",
			"imageUrl": "https://image.example.com",
			"lastOperation": {"type": "create", "state": "in progress"},
			"offeringName": "mariadb",
			"offeringVersion": "%s",
			"planName": "%s",
			"dashboardUrl": "",
			"parameters": {}
		}`, instanceId, instanceName, planId, version, planName)
		w.Write([]byte(jsonResp))
	}).Methods("GET")

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, "", instanceName, version, planName)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	// Assertions
	if createCalled == 0 {
		t.Fatal("CreateInstance API should have been called")
	}

	if getInstanceCalled == 0 {
		t.Fatal("GetInstance should have been called during wait")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to context timeout")
	}

	var stateAfterCreate Model
	diags := resp.State.Get(tc.Ctx, &stateAfterCreate)
	if diags.HasError() {
		t.Logf("Warnings getting state: %v", diags)
	}

	// Verify idempotency
	if stateAfterCreate.InstanceId.IsNull() || stateAfterCreate.InstanceId.ValueString() == "" {
		t.Fatal("BUG: InstanceId should be saved to state immediately after CreateInstance API succeeds")
	}

	// Verify all expected fields are set in state after failed wait
	AssertStateFieldEquals(t, "InstanceId", stateAfterCreate.InstanceId, types.StringValue(instanceId))
	AssertStateFieldEquals(t, "Id", stateAfterCreate.Id, types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)))
	AssertStateFieldEquals(t, "ProjectId", stateAfterCreate.ProjectId, types.StringValue(projectId))
	AssertStateFieldEquals(t, "Name", stateAfterCreate.Name, types.StringValue(instanceName))
	AssertStateFieldEquals(t, "Version", stateAfterCreate.Version, types.StringValue(version))
	AssertStateFieldEquals(t, "PlanName", stateAfterCreate.PlanName, types.StringValue(planName))

	// PlanId should be set by loadPlanId call
	AssertStateFieldEquals(t, "PlanId", stateAfterCreate.PlanId, types.StringValue(planId))
}

func TestCreate_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "instance-abc-123"
	instanceName := "my-test-instance"
	planId := "plan-123"
	planName := "stackit-mariadb-1.4.10-single"
	version := "1.4.10"
	dashboardUrl := "https://dashboard.example.com"

	createCalled := 0
	tc.SetupCreateInstanceHandler(instanceId, &createCalled)
	tc.SetupListOfferingsHandler(version, planId, planName)
	tc.SetupGetInstanceHandler(InstanceResponse{
		InstanceId:   instanceId,
		Name:         instanceName,
		PlanId:       planId,
		PlanName:     planName,
		Version:      version,
		DashboardUrl: dashboardUrl,
		Status:       "active",
	})

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, "", instanceName, version, planName)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	if createCalled == 0 {
		t.Fatal("CreateInstance API should have been called")
	}

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	// Verify all fields match what was returned from GetInstance handler
	AssertStateFieldEquals(t, "InstanceId", finalState.InstanceId, types.StringValue(instanceId))
	AssertStateFieldEquals(t, "Id", finalState.Id, types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)))
	AssertStateFieldEquals(t, "ProjectId", finalState.ProjectId, types.StringValue(projectId))
	AssertStateFieldEquals(t, "Name", finalState.Name, types.StringValue(instanceName))
	AssertStateFieldEquals(t, "PlanId", finalState.PlanId, types.StringValue(planId))
	AssertStateFieldEquals(t, "PlanName", finalState.PlanName, types.StringValue(planName))
	AssertStateFieldEquals(t, "Version", finalState.Version, types.StringValue(version))
	AssertStateFieldEquals(t, "DashboardUrl", finalState.DashboardUrl, types.StringValue(dashboardUrl))

	if finalState.CfGuid.IsNull() {
		t.Error("CfGuid should be set from API response")
	}
	if finalState.ImageUrl.IsNull() {
		t.Error("ImageUrl should be set from API response")
	}
}

func TestCreate_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	tc.Router.HandleFunc("/v1/projects/{projectId}/instances", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "Internal server error"}`))
	}).Methods("POST")

	tc.SetupListOfferingsHandler("1.4.10", "plan-123", "stackit-mariadb-1.4.10-single")

	schema := tc.GetSchema()
	model := CreateTestModel("test-project", "", "test-instance", "1.4.10", "stackit-mariadb-1.4.10-single")
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when API call fails")
	}

	// State should be empty since instance was never created
	var stateAfterCreate Model
	resp.State.Get(tc.Ctx, &stateAfterCreate)

	if !stateAfterCreate.InstanceId.IsNull() {
		t.Error("InstanceId should be null when API call fails")
	}
}

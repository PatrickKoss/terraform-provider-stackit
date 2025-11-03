package mariadb

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
	instanceId := "instance-abc-123"
	instanceName := "my-test-instance"
	oldPlanId := "old-plan-123"
	newPlanId := "new-plan-456"
	oldPlanName := "stackit-mariadb-1.4.10-single"
	newPlanName := "stackit-mariadb-1.4.10-replica"
	version := "1.4.10"
	dashboardUrl := "https://dashboard.example.com"

	// Setup mock handlers
	updateCalled := 0
	tc.SetupUpdateInstanceHandler(&updateCalled)

	plans := map[string]string{
		oldPlanId: oldPlanName,
		newPlanId: newPlanName,
	}
	tc.SetupListOfferingsHandlerMultiplePlans(version, plans)

	tc.SetupGetInstanceHandler(InstanceResponse{
		InstanceId:   instanceId,
		Name:         instanceName,
		PlanId:       newPlanId,
		PlanName:     newPlanName,
		Version:      version,
		DashboardUrl: dashboardUrl,
		Status:       "active",
	})

	// Prepare request
	schema := tc.GetSchema()

	currentState := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		Version:    types.StringValue(version),
		PlanId:     types.StringValue(oldPlanId),
		PlanName:   types.StringValue(oldPlanName),
		Parameters: types.ObjectNull(parametersTypes),
	}

	plannedState := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		Version:    types.StringValue(version),
		PlanId:     types.StringValue(newPlanId),
		PlanName:   types.StringValue(newPlanName),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := UpdateRequest(tc.Ctx, schema, currentState, plannedState)
	resp := UpdateResponse(tc.Ctx, schema, nil)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	if updateCalled == 0 {
		t.Fatal("PartialUpdateInstance API should have been called")
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

	// Verify all fields match the updated values from GetInstance
	AssertStateFieldEquals(t, "InstanceId", finalState.InstanceId, types.StringValue(instanceId))
	AssertStateFieldEquals(t, "Id", finalState.Id, types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)))
	AssertStateFieldEquals(t, "ProjectId", finalState.ProjectId, types.StringValue(projectId))
	AssertStateFieldEquals(t, "Name", finalState.Name, types.StringValue(instanceName))
	AssertStateFieldEquals(t, "PlanId", finalState.PlanId, types.StringValue(newPlanId))
	AssertStateFieldEquals(t, "PlanName", finalState.PlanName, types.StringValue(newPlanName))
	AssertStateFieldEquals(t, "Version", finalState.Version, types.StringValue(version))
	AssertStateFieldEquals(t, "DashboardUrl", finalState.DashboardUrl, types.StringValue(dashboardUrl))
}

func TestUpdate_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	instanceId := "instance-abc-123"
	instanceName := "my-test-instance"
	oldPlanId := "old-plan-123"
	newPlanId := "new-plan-456"
	oldPlanName := "stackit-mariadb-1.4.10-single"
	newPlanName := "stackit-mariadb-1.4.10-replica"
	version := "1.4.10"

	// Setup mock handlers
	updateCalled := 0
	tc.SetupUpdateInstanceHandler(&updateCalled)

	plans := map[string]string{
		oldPlanId: oldPlanName,
		newPlanId: newPlanName,
	}
	tc.SetupListOfferingsHandlerMultiplePlans(version, plans)

	// Setup GetInstance to simulate slow response (triggers timeout)
	getInstanceCalled := 0
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		getInstanceCalled++
		time.Sleep(150 * time.Millisecond) // Longer than context timeout
		w.Header().Set("Content-Type", "application/json")

		// API shows new plan (update is progressing on server side)
		jsonResp := fmt.Sprintf(`{
			"instanceId": "%s",
			"name": "%s",
			"planId": "%s",
			"status": "updating",
			"cfGuid": "cf-guid",
			"cfSpaceGuid": "cf-space-guid",
			"cfOrganizationGuid": "cf-org-guid",
			"imageUrl": "https://image.example.com",
			"lastOperation": {"type": "update", "state": "in progress"},
			"offeringName": "mariadb",
			"offeringVersion": "%s",
			"planName": "%s",
			"dashboardUrl": "https://dashboard.example.com",
			"parameters": {}
		}`, instanceId, instanceName, newPlanId, version, newPlanName)
		w.Write([]byte(jsonResp))
	}).Methods("GET")

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Prepare request
	schema := tc.GetSchema()

	currentState := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		Version:    types.StringValue(version),
		PlanId:     types.StringValue(oldPlanId),
		PlanName:   types.StringValue(oldPlanName),
		Parameters: types.ObjectNull(parametersTypes),
	}

	plannedState := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		Version:    types.StringValue(version),
		PlanId:     types.StringValue(newPlanId),
		PlanName:   types.StringValue(newPlanName),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := UpdateRequest(tc.Ctx, schema, currentState, plannedState)
	resp := UpdateResponse(tc.Ctx, schema, &currentState)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	if updateCalled == 0 {
		t.Fatal("PartialUpdateInstance API should have been called")
	}

	if getInstanceCalled == 0 {
		t.Fatal("GetInstance should have been called during wait")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to context timeout")
	}

	var stateAfterUpdate Model
	diags := resp.State.Get(tc.Ctx, &stateAfterUpdate)
	if diags.HasError() {
		t.Fatalf("Failed to get state after update: %v", diags.Errors())
	}

	if !stateAfterUpdate.PlanId.IsNull() {
		actualPlanId := stateAfterUpdate.PlanId.ValueString()
		if actualPlanId == newPlanId {
			t.Fatal("BUG: State has NEW PlanId even though update wait failed!")
		}
		AssertStateFieldEquals(t, "PlanId", stateAfterUpdate.PlanId, types.StringValue(oldPlanId))
	}

	if !stateAfterUpdate.PlanName.IsNull() {
		actualPlanName := stateAfterUpdate.PlanName.ValueString()
		if actualPlanName == newPlanName {
			t.Fatal("BUG: State has NEW PlanName even though update wait failed!")
		}
		AssertStateFieldEquals(t, "PlanName", stateAfterUpdate.PlanName, types.StringValue(oldPlanName))
	}

	AssertStateFieldEquals(t, "Id", stateAfterUpdate.Id, currentState.Id)
	AssertStateFieldEquals(t, "ProjectId", stateAfterUpdate.ProjectId, currentState.ProjectId)
	AssertStateFieldEquals(t, "InstanceId", stateAfterUpdate.InstanceId, currentState.InstanceId)
	AssertStateFieldEquals(t, "Name", stateAfterUpdate.Name, currentState.Name)
	AssertStateFieldEquals(t, "Version", stateAfterUpdate.Version, currentState.Version)
}

func TestUpdate_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Setup mock handlers
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "Internal server error"}`))
	}).Methods("PATCH")

	tc.SetupListOfferingsHandlerMultiplePlans("1.4.10", map[string]string{
		"old-plan": "old-plan-name",
		"new-plan": "new-plan-name",
	})

	// Prepare request
	schema := tc.GetSchema()

	currentState := Model{
		Id:         types.StringValue("test-project,test-instance"),
		ProjectId:  types.StringValue("test-project"),
		InstanceId: types.StringValue("test-instance"),
		Name:       types.StringValue("test-name"),
		Version:    types.StringValue("1.4.10"),
		PlanId:     types.StringValue("old-plan"),
		PlanName:   types.StringValue("old-plan-name"),
		Parameters: types.ObjectNull(parametersTypes),
	}

	plannedState := Model{
		Id:         types.StringValue("test-project,test-instance"),
		ProjectId:  types.StringValue("test-project"),
		InstanceId: types.StringValue("test-instance"),
		Name:       types.StringValue("test-name"),
		Version:    types.StringValue("1.4.10"),
		PlanId:     types.StringValue("new-plan"),
		PlanName:   types.StringValue("new-plan-name"),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := UpdateRequest(tc.Ctx, schema, currentState, plannedState)
	resp := UpdateResponse(tc.Ctx, schema, nil)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when API call fails")
	}
}

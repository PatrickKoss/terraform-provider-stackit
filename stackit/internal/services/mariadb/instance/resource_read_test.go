package mariadb

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRead_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	instanceId := "instance-abc-123"
	instanceName := "my-test-instance"
	planId := "plan-123"
	planName := "stackit-mariadb-1.4.10-single"
	version := "1.4.10"
	dashboardUrl := "https://dashboard.example.com"

	tc.SetupGetInstanceHandler(InstanceResponse{
		InstanceId:   instanceId,
		Name:         instanceName,
		PlanId:       planId,
		PlanName:     planName,
		Version:      version,
		DashboardUrl: dashboardUrl,
		Status:       "active",
	})

	tc.SetupListOfferingsHandler(version, planId, planName)

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		// Other fields may be outdated or null
		Name:       types.StringNull(),
		Version:    types.StringNull(),
		PlanId:     types.StringNull(),
		PlanName:   types.StringNull(),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	var refreshedState Model
	diags := resp.State.Get(tc.Ctx, &refreshedState)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	AssertStateFieldEquals(t, "InstanceId", refreshedState.InstanceId, types.StringValue(instanceId))
	AssertStateFieldEquals(t, "Id", refreshedState.Id, types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)))
	AssertStateFieldEquals(t, "ProjectId", refreshedState.ProjectId, types.StringValue(projectId))
	AssertStateFieldEquals(t, "Name", refreshedState.Name, types.StringValue(instanceName))
	AssertStateFieldEquals(t, "PlanId", refreshedState.PlanId, types.StringValue(planId))
	AssertStateFieldEquals(t, "PlanName", refreshedState.PlanName, types.StringValue(planName))
	AssertStateFieldEquals(t, "Version", refreshedState.Version, types.StringValue(version))
	AssertStateFieldEquals(t, "DashboardUrl", refreshedState.DashboardUrl, types.StringValue(dashboardUrl))

	if refreshedState.CfGuid.IsNull() {
		t.Error("CfGuid should be set from API response")
	}
	if refreshedState.ImageUrl.IsNull() {
		t.Error("ImageUrl should be set from API response")
	}
}

func TestRead_InstanceNotFound(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	instanceId := "non-existent-instance"

	// Setup GetInstance to return 404
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Instance not found"}`))
	}).Methods("GET")

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should not error when instance not found, but got errors: %v", resp.Diagnostics.Errors())
	}
}

func TestRead_InstanceGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	instanceId := "gone-instance"

	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		w.Write([]byte(`{"message": "Instance has been deleted"}`))
	}).Methods("GET")

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should not error when instance is gone, but got errors: %v", resp.Diagnostics.Errors())
	}
}

func TestRead_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Setup GetInstance to return 500 error
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "Internal server error"}`))
	}).Methods("GET")

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue("test-project,test-instance"),
		ProjectId:  types.StringValue("test-project"),
		InstanceId: types.StringValue("test-instance"),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when API call fails")
	}
}

func TestRead_DetectDrift(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	instanceId := "instance-abc-123"
	instanceName := "my-test-instance"
	oldPlanId := "old-plan-123"
	newPlanId := "new-plan-456" // Changed in cloud
	oldPlanName := "stackit-mariadb-1.4.10-single"
	newPlanName := "stackit-mariadb-1.4.10-replica" // Changed in cloud
	version := "1.4.10"

	tc.SetupGetInstanceHandler(InstanceResponse{
		InstanceId:   instanceId,
		Name:         instanceName,
		PlanId:       newPlanId, // Drift detected!
		PlanName:     newPlanName,
		Version:      version,
		DashboardUrl: "https://dashboard.example.com",
		Status:       "active",
	})

	plans := map[string]string{
		oldPlanId: oldPlanName,
		newPlanId: newPlanName,
	}
	tc.SetupListOfferingsHandlerMultiplePlans(version, plans)

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		Version:    types.StringValue(version),
		PlanId:     types.StringValue(oldPlanId),   // Old value
		PlanName:   types.StringValue(oldPlanName), // Old value
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	var refreshedState Model
	diags := resp.State.Get(tc.Ctx, &refreshedState)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	AssertStateFieldEquals(t, "PlanId", refreshedState.PlanId, types.StringValue(newPlanId))
	AssertStateFieldEquals(t, "PlanName", refreshedState.PlanName, types.StringValue(newPlanName))
}

package mariadb

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

	projectId := "test-project-123"
	instanceId := "instance-abc-123"
	instanceName := "my-test-instance"
	planId := "plan-123"
	planName := "stackit-mariadb-1.4.10-single"
	version := "1.4.10"

	deleteCalled := 0
	tc.SetupDeleteInstanceHandler(&deleteCalled)

	// Setup GetInstance - return 410 Gone (instance was deleted)
	// The delete wait handler polls GetInstance and succeeds when it gets 404 or 410
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Instance is deleted - return 410 Gone
		w.WriteHeader(http.StatusGone)
		w.Write([]byte(`{"message": "Instance has been deleted"}`))
	}).Methods("GET")

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		Version:    types.StringValue(version),
		PlanId:     types.StringValue(planId),
		PlanName:   types.StringValue(planName),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	if deleteCalled == 0 {
		t.Fatal("DeleteInstance API should have been called")
	}

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}
}

func TestDelete_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	instanceId := "instance-abc-123"
	instanceName := "my-test-instance"
	planId := "plan-123"
	planName := "stackit-mariadb-1.4.10-single"
	version := "1.4.10"

	deleteCalled := 0
	tc.SetupDeleteInstanceHandler(&deleteCalled)

	getInstanceCalled := 0
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		getInstanceCalled++
		time.Sleep(150 * time.Millisecond) // Longer than context timeout
		w.Header().Set("Content-Type", "application/json")

		// Instance is still being deleted
		jsonResp := fmt.Sprintf(`{
			"instanceId": "%s",
			"name": "%s",
			"planId": "%s",
			"status": "deleting",
			"cfGuid": "cf-guid",
			"cfSpaceGuid": "cf-space-guid",
			"cfOrganizationGuid": "cf-org-guid",
			"imageUrl": "https://image.example.com",
			"lastOperation": {"type": "delete", "state": "in progress"},
			"offeringName": "mariadb",
			"offeringVersion": "%s",
			"planName": "%s",
			"dashboardUrl": "",
			"parameters": {}
		}`, instanceId, instanceName, planId, version, planName)
		w.Write([]byte(jsonResp))
	}).Methods("GET")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, instanceId)),
		ProjectId:  types.StringValue(projectId),
		InstanceId: types.StringValue(instanceId),
		Name:       types.StringValue(instanceName),
		Version:    types.StringValue(version),
		PlanId:     types.StringValue(planId),
		PlanName:   types.StringValue(planName),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, &state)

	tc.Resource.Delete(tc.Ctx, req, resp)

	if deleteCalled == 0 {
		t.Fatal("DeleteInstance API should have been called")
	}

	if getInstanceCalled == 0 {
		t.Fatal("GetInstance should have been called during wait")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to context timeout")
	}

	var stateAfterDelete Model
	diags := resp.State.Get(tc.Ctx, &stateAfterDelete)
	if diags.HasError() {
		t.Fatalf("Failed to get state after delete: %v", diags.Errors())
	}

	AssertStateFieldEquals(t, "InstanceId", stateAfterDelete.InstanceId, state.InstanceId)
	AssertStateFieldEquals(t, "Id", stateAfterDelete.Id, state.Id)
	AssertStateFieldEquals(t, "ProjectId", stateAfterDelete.ProjectId, state.ProjectId)
	AssertStateFieldEquals(t, "Name", stateAfterDelete.Name, state.Name)
	AssertStateFieldEquals(t, "Version", stateAfterDelete.Version, state.Version)
	AssertStateFieldEquals(t, "PlanId", stateAfterDelete.PlanId, state.PlanId)
	AssertStateFieldEquals(t, "PlanName", stateAfterDelete.PlanName, state.PlanName)
}

func TestDelete_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "Internal server error"}`))
	}).Methods("DELETE")

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue("test-project,test-instance"),
		ProjectId:  types.StringValue("test-project"),
		InstanceId: types.StringValue("test-instance"),
		Name:       types.StringValue("test-name"),
		Version:    types.StringValue("1.4.10"),
		PlanId:     types.StringValue("plan-123"),
		PlanName:   types.StringValue("plan-name"),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when API call fails")
	}
}

func TestDelete_InstanceAlreadyDeleted(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	deleteCalled := 0
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		deleteCalled++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Instance not found"}`))
	}).Methods("DELETE")

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue("test-project,test-instance"),
		ProjectId:  types.StringValue("test-project"),
		InstanceId: types.StringValue("test-instance"),
		Name:       types.StringValue("test-name"),
		Version:    types.StringValue("1.4.10"),
		PlanId:     types.StringValue("plan-123"),
		PlanName:   types.StringValue("plan-name"),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	if deleteCalled == 0 {
		t.Fatal("DeleteInstance API should have been called")
	}

	// Delete should succeed for idempotency - instance already deleted
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed when instance is already deleted (404), but got errors: %v", resp.Diagnostics.Errors())
	}
}

func TestDelete_InstanceGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Setup mock handlers - DeleteInstance returns 410 Gone
	deleteCalled := 0
	tc.Router.HandleFunc("/v1/projects/{projectId}/instances/{instanceId}", func(w http.ResponseWriter, r *http.Request) {
		deleteCalled++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		w.Write([]byte(`{"message": "Instance has been deleted"}`))
	}).Methods("DELETE")

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue("test-project,test-instance"),
		ProjectId:  types.StringValue("test-project"),
		InstanceId: types.StringValue("test-instance"),
		Name:       types.StringValue("test-name"),
		Version:    types.StringValue("1.4.10"),
		PlanId:     types.StringValue("plan-123"),
		PlanName:   types.StringValue("plan-name"),
		Parameters: types.ObjectNull(parametersTypes),
	}

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	if deleteCalled == 0 {
		t.Fatal("DeleteInstance API should have been called")
	}

	// Delete should succeed for idempotency - instance already gone
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed when instance is gone (410), but got errors: %v", resp.Diagnostics.Errors())
	}
}

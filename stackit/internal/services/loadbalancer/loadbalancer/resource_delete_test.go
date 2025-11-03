package loadbalancer

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
	region := "eu01"
	lbName := "my-test-lb"

	deleteCalled := 0
	tc.SetupDeleteLoadBalancerHandler(&deleteCalled)

	// Setup GetLoadBalancer to return 404 Not Found after deletion
	// The delete wait handler expects 404 to confirm successful deletion
	tc.Router.HandleFunc("/v2/projects/{projectId}/regions/{region}/load-balancers/{name}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Load balancer not found"}`))
	}).Methods("GET")

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, region, lbName, "1.2.3.4")
	model.Id = types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, region, lbName))
	req := DeleteRequest(tc.Ctx, schema, model)
	resp := DeleteResponse(tc.Ctx, schema, &model)

	tc.Resource.Delete(tc.Ctx, req, resp)

	if deleteCalled == 0 {
		t.Fatal("DeleteLoadBalancer API should have been called")
	}

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	t.Log("SUCCESS: Load balancer deleted successfully")
}

func TestDelete_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	region := "eu01"
	lbName := "my-test-lb"

	deleteCalled := 0
	tc.SetupDeleteLoadBalancerHandler(&deleteCalled)

	// Setup GetLoadBalancer to simulate slow response (triggers timeout)
	tc.Router.HandleFunc("/v2/projects/{projectId}/regions/{region}/load-balancers/{name}", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond) // Longer than context timeout
		w.Header().Set("Content-Type", "application/json")
		jsonResp := fmt.Sprintf(`{
			"name": "%s",
			"status": "deleting"
		}`, lbName)
		w.Write([]byte(jsonResp))
	}).Methods("GET")

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Prepare request with current state
	schema := tc.GetSchema()
	state := CreateTestModel(projectId, region, lbName, "1.2.3.4")
	state.Id = types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, region, lbName))
	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, &state)

	// Execute Delete
	tc.Resource.Delete(tc.Ctx, req, resp)

	// Assertions
	if deleteCalled == 0 {
		t.Fatal("DeleteLoadBalancer API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to context timeout")
	}

	// Verify helpful error message
	errorMsgs := resp.Diagnostics.Errors()
	if len(errorMsgs) == 0 {
		t.Fatal("Expected error messages")
	}
	errorMsg := errorMsgs[0].Summary() + " " + errorMsgs[0].Detail()
	if len(errorMsg) < 50 {
		t.Error("Error message should be helpful and informative")
	}

	// After timeout, verify state is NOT removed (resource still tracked)
	var stateAfterDelete Model
	diags := resp.State.Get(tc.Ctx, &stateAfterDelete)
	if diags.HasError() {
		t.Fatalf("Failed to get state after delete: %v", diags.Errors())
	}

	// Verify all fields match original state (resource still tracked)
	AssertStateFieldEquals(t, "Name", stateAfterDelete.Name, state.Name)
	AssertStateFieldEquals(t, "Id", stateAfterDelete.Id, state.Id)
	AssertStateFieldEquals(t, "ProjectId", stateAfterDelete.ProjectId, state.ProjectId)
	AssertStateFieldEquals(t, "Region", stateAfterDelete.Region, state.Region)

	t.Log("GOOD: State preserved after delete timeout - user can retry")
}

func TestDelete_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	tc.Router.HandleFunc("/v2/projects/{projectId}/regions/{region}/load-balancers/{name}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "Internal server error"}`))
	}).Methods("DELETE")

	schema := tc.GetSchema()
	model := CreateTestModel("test-project", "eu01", "test-lb", "1.2.3.4")
	model.Id = types.StringValue("test-project,eu01,test-lb")
	req := DeleteRequest(tc.Ctx, schema, model)
	resp := DeleteResponse(tc.Ctx, schema, &model)

	tc.Resource.Delete(tc.Ctx, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when API call fails")
	}
}

func TestDelete_ResourceAlreadyDeleted(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Setup DeleteLoadBalancer to return 404 Not Found
	tc.Router.HandleFunc("/v2/projects/{projectId}/regions/{region}/load-balancers/{name}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Load balancer not found"}`))
	}).Methods("DELETE")

	schema := tc.GetSchema()
	model := CreateTestModel("test-project", "eu01", "test-lb", "1.2.3.4")
	model.Id = types.StringValue("test-project,eu01,test-lb")
	req := DeleteRequest(tc.Ctx, schema, model)
	resp := DeleteResponse(tc.Ctx, schema, &model)

	tc.Resource.Delete(tc.Ctx, req, resp)

	// Should NOT error - idempotent delete
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed for idempotency (404), but got errors: %v", resp.Diagnostics.Errors())
	}

	t.Log("SUCCESS: Delete is idempotent - 404 treated as success")
}

func TestDelete_ResourceGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Setup DeleteLoadBalancer to return 410 Gone
	tc.Router.HandleFunc("/v2/projects/{projectId}/regions/{region}/load-balancers/{name}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		w.Write([]byte(`{"message": "Load balancer is gone"}`))
	}).Methods("DELETE")

	schema := tc.GetSchema()
	model := CreateTestModel("test-project", "eu01", "test-lb", "1.2.3.4")
	model.Id = types.StringValue("test-project,eu01,test-lb")
	req := DeleteRequest(tc.Ctx, schema, model)
	resp := DeleteResponse(tc.Ctx, schema, &model)

	tc.Resource.Delete(tc.Ctx, req, resp)

	// Should NOT error - idempotent delete
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete should succeed for idempotency (410), but got errors: %v", resp.Diagnostics.Errors())
	}

	t.Log("SUCCESS: Delete is idempotent - 410 treated as success")
}

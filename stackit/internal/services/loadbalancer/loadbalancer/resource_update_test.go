package loadbalancer

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUpdate_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	region := "eu01"
	lbName := "my-test-lb"
	externalAddress := "1.2.3.4"

	updateCalled := 0

	// Add catch-all handler to log all requests
	tc.Router.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Request: %s %s", r.Method, r.URL.Path)

		// Handle PUT to update target pool (SDK uses PUT not PATCH)
		if r.Method == "PUT" && strings.Contains(r.URL.Path, "target-pools") {
			updateCalled++
			t.Logf("UpdateTargetPool called!")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte("{}"))
			return
		}

		// Handle GET to load balancer
		if r.Method == "GET" && strings.Contains(r.URL.Path, "load-balancers") {
			t.Logf("GetLoadBalancer called!")
			w.Header().Set("Content-Type", "application/json")
			jsonResp := fmt.Sprintf(`{
				"name": "%s",
				"externalAddress": "%s",
				"privateAddress": "10.0.0.1",
				"status": "STATUS_READY"
			}`, lbName, externalAddress)
			w.Write([]byte(jsonResp))
			return
		}

		t.Logf("Unhandled request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	})

	schema := tc.GetSchema()
	model := CreateTestModel(projectId, region, lbName, externalAddress)
	model.Id = types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, region, lbName))
	req := UpdateRequest(tc.Ctx, schema, model)
	resp := UpdateResponse(tc.Ctx, schema, &model)

	tc.Resource.Update(tc.Ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Logf("Update errors: %v", resp.Diagnostics.Errors())
	}

	if updateCalled == 0 {
		t.Fatal("UpdateTargetPool API should have been called")
	}

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	AssertStateFieldEquals(t, "Name", finalState.Name, types.StringValue(lbName))
	AssertStateFieldEquals(t, "ExternalAddress", finalState.ExternalAddress, types.StringValue(externalAddress))

	t.Log("SUCCESS: Load balancer updated successfully")
}

func TestUpdate_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Setup UpdateTargetPool to fail
	tc.Router.HandleFunc("/v2/projects/{projectId}/regions/{region}/load-balancers/{name}/target-pools/{targetPoolName}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "Internal server error"}`))
	}).Methods("PATCH")

	schema := tc.GetSchema()
	model := CreateTestModel("test-project", "eu01", "test-lb", "1.2.3.4")
	model.Id = types.StringValue("test-project,eu01,test-lb")
	req := UpdateRequest(tc.Ctx, schema, model)
	resp := UpdateResponse(tc.Ctx, schema, &model)

	tc.Resource.Update(tc.Ctx, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when API call fails")
	}
}

func TestUpdate_GetAfterUpdateFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Update succeeds but Get fails
	updateCalled := 0

	// Add catch-all handler
	tc.Router.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle PUT to update target pool (SDK uses PUT not PATCH)
		if r.Method == "PUT" && strings.Contains(r.URL.Path, "target-pools") {
			updateCalled++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte("{}"))
			return
		}

		// Handle GET - return error
		if r.Method == "GET" && strings.Contains(r.URL.Path, "load-balancers") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"message": "Internal server error"}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	schema := tc.GetSchema()
	currentState := CreateTestModel("test-project", "eu01", "test-lb", "1.2.3.4")
	currentState.Id = types.StringValue("test-project,eu01,test-lb")
	req := UpdateRequest(tc.Ctx, schema, currentState)
	resp := UpdateResponse(tc.Ctx, schema, &currentState)

	tc.Resource.Update(tc.Ctx, req, resp)

	if updateCalled == 0 {
		t.Fatal("UpdateTargetPool should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when GetLoadBalancer fails")
	}

	// State should still have old values since Get failed
	var stateAfterUpdate Model
	diags := resp.State.Get(tc.Ctx, &stateAfterUpdate)
	if diags.HasError() {
		t.Logf("Warnings getting state: %v", diags)
	}

	// Verify state has old values
	AssertStateFieldEquals(t, "Name", stateAfterUpdate.Name, currentState.Name)
	AssertStateFieldEquals(t, "Id", stateAfterUpdate.Id, currentState.Id)

	t.Log("State preserved with old values when GetLoadBalancer fails after update")
}

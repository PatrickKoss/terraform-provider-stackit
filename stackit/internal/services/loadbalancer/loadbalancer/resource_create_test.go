package loadbalancer

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestCreate_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	region := "eu01"
	lbName := "my-test-lb"
	externalAddress := "1.2.3.4"

	// Add catch-all handler to log all requests
	createCalled := 0
	tc.Router.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Request: %s %s", r.Method, r.URL.Path)

		// Handle POST to create load balancer
		if r.Method == "POST" && strings.Contains(r.URL.Path, "load-balancers") {
			createCalled++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			jsonResp := fmt.Sprintf(`{"name": "%s"}`, lbName)
			w.Write([]byte(jsonResp))
			return
		}

		// Handle GET requests
		if r.Method == "GET" && strings.Contains(r.URL.Path, "load-balancers") {
			time.Sleep(150 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			jsonResp := fmt.Sprintf(`{
				"name": "%s",
				"externalAddress": "%s",
				"privateAddress": "10.0.0.1",
				"status": "creating"
			}`, lbName, externalAddress)
			w.Write([]byte(jsonResp))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	// Don't call SetupCreateLoadBalancerHandler since we're using catch-all

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, region, lbName, externalAddress)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	// Debug output
	if resp.Diagnostics.HasError() {
		t.Logf("Errors: %v", resp.Diagnostics.Errors())
		for _, err := range resp.Diagnostics.Errors() {
			t.Logf("Error: %s - %s", err.Summary(), err.Detail())
		}
	}

	// Assertions
	if createCalled == 0 {
		t.Fatal("CreateLoadBalancer API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to context timeout")
	}

	var stateAfterCreate Model
	diags := resp.State.Get(tc.Ctx, &stateAfterCreate)
	if diags.HasError() {
		t.Logf("Warnings getting state: %v", diags)
	}

	// Verify idempotency fix - Name should be saved even though wait failed
	if stateAfterCreate.Name.IsNull() || stateAfterCreate.Name.ValueString() == "" {
		t.Fatal("BUG: Name should be saved to state immediately after CreateLoadBalancer API succeeds")
	}

	// Verify all expected fields are set in state after failed wait
	AssertStateFieldEquals(t, "Name", stateAfterCreate.Name, types.StringValue(lbName))
	AssertStateFieldEquals(t, "Id", stateAfterCreate.Id, types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, region, lbName)))
	AssertStateFieldEquals(t, "ProjectId", stateAfterCreate.ProjectId, types.StringValue(projectId))
	AssertStateFieldEquals(t, "Region", stateAfterCreate.Region, types.StringValue(region))

	t.Log("GOOD: Name and Id saved even though wait failed - idempotency guaranteed")
}

func TestCreate_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	region := "eu01"
	lbName := "my-test-lb"
	externalAddress := "1.2.3.4"
	privateAddress := "10.0.0.1"

	createCalled := 0
	tc.SetupCreateLoadBalancerHandler(lbName, &createCalled)
	tc.SetupGetLoadBalancerHandler(LoadBalancerResponse{
		Name:            lbName,
		ExternalAddress: externalAddress,
		PrivateAddress:  privateAddress,
		Status:          "STATUS_READY",
	})

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, region, lbName, externalAddress)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	if createCalled == 0 {
		t.Fatal("CreateLoadBalancer API should have been called")
	}

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	// Verify all fields match what was returned from GetLoadBalancer handler
	AssertStateFieldEquals(t, "Name", finalState.Name, types.StringValue(lbName))
	AssertStateFieldEquals(t, "Id", finalState.Id, types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, region, lbName)))
	AssertStateFieldEquals(t, "ProjectId", finalState.ProjectId, types.StringValue(projectId))
	AssertStateFieldEquals(t, "Region", finalState.Region, types.StringValue(region))
	AssertStateFieldEquals(t, "ExternalAddress", finalState.ExternalAddress, types.StringValue(externalAddress))
	AssertStateFieldEquals(t, "PrivateAddress", finalState.PrivateAddress, types.StringValue(privateAddress))

	t.Log("SUCCESS: All state fields correctly populated")
}

func TestCreate_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	tc.Router.HandleFunc("/v2/projects/{projectId}/regions/{region}/load-balancers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "Internal server error"}`))
	}).Methods("POST")

	schema := tc.GetSchema()
	model := CreateTestModel("test-project", "eu01", "test-lb", "1.2.3.4")
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when API call fails")
	}

	// State should be empty since load balancer was never created
	var stateAfterCreate Model
	resp.State.Get(tc.Ctx, &stateAfterCreate)

	if !stateAfterCreate.Name.IsNull() {
		t.Error("Name should be null when API call fails")
	}
}

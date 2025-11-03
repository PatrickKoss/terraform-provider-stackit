package cdn

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
	distributionId := "dist-abc-123"
	customDomainName := "example.com"

	// Setup mock handlers
	putCalled := 0
	tc.SetupPutCustomDomainHandler(&putCalled)

	// Setup GetCustomDomain to simulate slow response (triggers timeout)
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}/customDomains/{name}", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		jsonResp := fmt.Sprintf(`{
			"customDomain": {
				"name": "%s",
				"status": "CREATING"
			},
			"certificate": {
				"type": "managed"
			}
		}`, customDomainName)
		w.Write([]byte(jsonResp))
	}).Methods("GET")

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Prepare request
	schema := tc.GetSchema()
	model := BuildTestModel(projectId, distributionId, customDomainName)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	// Assertions
	if putCalled == 0 {
		t.Fatal("PutCustomDomain API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to context timeout")
	}

	// CRITICAL: Verify ID was saved
	var stateAfterCreate CustomDomainModel
	diags := resp.State.Get(context.Background(), &stateAfterCreate)
	if diags.HasError() {
		t.Logf("Warnings getting state: %v", diags)
	}

	// Verify idempotency fix
	if stateAfterCreate.ID.IsNull() || stateAfterCreate.ID.ValueString() == "" {
		t.Fatal("BUG: ID should be saved to state immediately after API succeeds")
	}

	expectedId := fmt.Sprintf("%s,%s,%s", projectId, distributionId, customDomainName)
	AssertStateFieldEquals(t, "ID", stateAfterCreate.ID, types.StringValue(expectedId))
	AssertStateFieldEquals(t, "ProjectId", stateAfterCreate.ProjectId, types.StringValue(projectId))
	AssertStateFieldEquals(t, "DistributionId", stateAfterCreate.DistributionId, types.StringValue(distributionId))
	AssertStateFieldEquals(t, "Name", stateAfterCreate.Name, types.StringValue(customDomainName))

	t.Log("GOOD: ID saved even though wait failed - idempotency guaranteed")
}

func TestCreate_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	distributionId := "dist-abc-123"
	customDomainName := "example.com"

	// Setup mock handlers
	putCalled := 0
	tc.SetupPutCustomDomainHandler(&putCalled)
	tc.SetupGetCustomDomainHandler(customDomainName, "ACTIVE")

	// Prepare request
	schema := tc.GetSchema()
	model := BuildTestModel(projectId, distributionId, customDomainName)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	t.Logf("About to call Create with model: ProjectId=%s, DistributionId=%s, Name=%s",
		model.ProjectId.ValueString(), model.DistributionId.ValueString(), model.Name.ValueString())

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	t.Logf("Create finished. PutCalled=%d, HasError=%v", putCalled, resp.Diagnostics.HasError())
	if resp.Diagnostics.HasError() {
		t.Logf("Errors: %v", resp.Diagnostics.Errors())
	}

	// Assertions
	if putCalled == 0 {
		t.Fatal("PutCustomDomain API should have been called")
	}

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Extract final state
	var finalState CustomDomainModel
	diags := resp.State.Get(tc.Ctx, &finalState)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	// Verify all fields
	expectedId := fmt.Sprintf("%s,%s,%s", projectId, distributionId, customDomainName)
	AssertStateFieldEquals(t, "ID", finalState.ID, types.StringValue(expectedId))
	AssertStateFieldEquals(t, "ProjectId", finalState.ProjectId, types.StringValue(projectId))
	AssertStateFieldEquals(t, "DistributionId", finalState.DistributionId, types.StringValue(distributionId))
	AssertStateFieldEquals(t, "Name", finalState.Name, types.StringValue(customDomainName))
	AssertStateFieldEquals(t, "Status", finalState.Status, types.StringValue("ACTIVE"))

	// Verify certificate is null (managed certificate)
	if !finalState.Certificate.IsNull() {
		t.Error("Certificate should be null for managed certificate")
	}

	t.Log("SUCCESS: All state fields correctly populated")
}

func TestCreate_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	distributionId := "dist-abc-123"
	customDomainName := "example.com"

	// Setup mock handlers - PutCustomDomain returns error
	putCalled := 0
	tc.SetupPutCustomDomainHandlerError(&putCalled)

	// Prepare request
	schema := tc.GetSchema()
	model := BuildTestModel(projectId, distributionId, customDomainName)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	// Assertions
	if putCalled == 0 {
		t.Fatal("PutCustomDomain API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from API call")
	}

	// Verify ID is null (nothing created)
	var stateAfterCreate CustomDomainModel
	diags := resp.State.Get(tc.Ctx, &stateAfterCreate)
	if diags.HasError() {
		t.Logf("Diagnostics getting state: %v", diags)
	}

	if !stateAfterCreate.ID.IsNull() {
		t.Error("ID should be null when API call fails")
	}

	t.Log("CORRECT: No state saved when API call fails")
}

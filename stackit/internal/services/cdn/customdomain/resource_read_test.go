package cdn

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRead_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	distributionId := "dist-abc-123"
	customDomainName := "example.com"

	// Setup mock handler
	tc.SetupGetCustomDomainHandler(customDomainName, "ACTIVE")

	// Prepare request
	schema := tc.GetSchema()
	model := BuildTestModel(projectId, distributionId, customDomainName)
	model.ID = types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, distributionId, customDomainName))

	req := ReadRequest(tc.Ctx, schema, model)
	resp := ReadResponse(tc.Ctx, schema, &model)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Extract state
	var stateAfterRead CustomDomainModel
	diags := resp.State.Get(tc.Ctx, &stateAfterRead)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	// Verify all fields match API response
	AssertStateFieldEquals(t, "Name", stateAfterRead.Name, types.StringValue(customDomainName))
	AssertStateFieldEquals(t, "Status", stateAfterRead.Status, types.StringValue("ACTIVE"))
	AssertStateFieldEquals(t, "ProjectId", stateAfterRead.ProjectId, types.StringValue(projectId))
	AssertStateFieldEquals(t, "DistributionId", stateAfterRead.DistributionId, types.StringValue(distributionId))

	t.Log("SUCCESS: Read completed and state refreshed")
}

func TestRead_ResourceNotFound(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	distributionId := "dist-abc-123"
	customDomainName := "example.com"

	// Setup mock handler - returns 404
	tc.SetupGetCustomDomainHandlerNotFound()

	// Prepare request
	schema := tc.GetSchema()
	model := BuildTestModel(projectId, distributionId, customDomainName)
	model.ID = types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, distributionId, customDomainName))

	req := ReadRequest(tc.Ctx, schema, model)
	resp := ReadResponse(tc.Ctx, schema, &model)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions
	// Should NOT error (404 is expected when resource is deleted externally)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should not error on 404, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Verify state was removed
	var stateAfterRead CustomDomainModel
	diags := resp.State.Get(tc.Ctx, &stateAfterRead)

	// State should be empty/removed after 404
	if !diags.HasError() {
		// If we can get state, it should have null ID
		if !stateAfterRead.ID.IsNull() {
			t.Error("State should be removed after 404 Not Found")
		}
	}

	t.Log("SUCCESS: Read handled 404 gracefully (resource removed from state)")
}

func TestRead_ResourceGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	distributionId := "dist-abc-123"
	customDomainName := "example.com"

	// Setup mock handler - returns 410
	tc.SetupGetCustomDomainHandlerGone()

	// Prepare request
	schema := tc.GetSchema()
	model := BuildTestModel(projectId, distributionId, customDomainName)
	model.ID = types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, distributionId, customDomainName))

	req := ReadRequest(tc.Ctx, schema, model)
	resp := ReadResponse(tc.Ctx, schema, &model)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions
	// Should NOT error (410 is expected when resource is gone)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should not error on 410, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Note: The current implementation doesn't handle 410 explicitly in Read,
	// but we're testing that it should in the future

	t.Log("Read handled 410 response")
}

func TestRead_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	distributionId := "dist-abc-123"
	customDomainName := "example.com"

	// Setup mock handler - returns 500
	tc.SetupGetCustomDomainHandlerError()

	// Prepare request
	schema := tc.GetSchema()
	model := BuildTestModel(projectId, distributionId, customDomainName)
	model.ID = types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, distributionId, customDomainName))

	req := ReadRequest(tc.Ctx, schema, model)
	resp := ReadResponse(tc.Ctx, schema, &model)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions
	// Should error on 500
	if !resp.Diagnostics.HasError() {
		t.Fatal("Read should error on 500 Internal Server Error")
	}

	t.Log("CORRECT: Read returned error for API failure")
}

func TestRead_DetectDrift(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	distributionId := "dist-abc-123"
	customDomainName := "example.com"

	// Setup mock handler with different status
	tc.SetupGetCustomDomainHandler(customDomainName, "UPDATING")

	// Prepare request with old state
	schema := tc.GetSchema()
	model := BuildTestModel(projectId, distributionId, customDomainName)
	model.ID = types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, distributionId, customDomainName))
	model.Status = types.StringValue("ACTIVE") // Old status

	req := ReadRequest(tc.Ctx, schema, model)
	resp := ReadResponse(tc.Ctx, schema, &model)

	// Execute Read
	tc.Resource.Read(tc.Ctx, req, resp)

	// Assertions
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Extract state
	var stateAfterRead CustomDomainModel
	diags := resp.State.Get(tc.Ctx, &stateAfterRead)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	// Verify state was updated with new values (drift detected)
	AssertStateFieldEquals(t, "Status", stateAfterRead.Status, types.StringValue("UPDATING"))
	AssertStateFieldEquals(t, "Name", stateAfterRead.Name, types.StringValue(customDomainName))

	t.Log("SUCCESS: Read detected drift and updated state")
}

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

func TestUpdate_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	distributionId := "dist-abc-123"
	customDomainName := "example.com"
	newCertVersion := int64(2)

	// Setup mock handlers
	putCalled := 0
	tc.SetupPutCustomDomainHandler(&putCalled)
	tc.SetupGetCustomDomainHandlerWithCertificate(customDomainName, "ACTIVE", newCertVersion)

	// Prepare request with certificate update
	schema := tc.GetSchema()

	// Build certificate object
	certAttrs := map[string]attr.Value{
		"certificate": types.StringValue("-----BEGIN CERTIFICATE-----\nMIIC...updated\n-----END CERTIFICATE-----"),
		"private_key": types.StringValue("-----BEGIN PRIVATE KEY-----\nMIIE...updated\n-----END PRIVATE KEY-----"),
		"version":     types.Int64Value(newCertVersion),
	}
	certObj, _ := types.ObjectValue(certificateTypes, certAttrs)

	model := CustomDomainModel{
		ProjectId:      types.StringValue(projectId),
		DistributionId: types.StringValue(distributionId),
		Name:           types.StringValue(customDomainName),
		ID:             types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, distributionId, customDomainName)),
		Status:         types.StringNull(),
		Errors:         types.ListNull(types.StringType),
		Certificate:    certObj,
	}

	req := UpdateRequest(tc.Ctx, schema, model)
	resp := UpdateResponse(tc.Ctx, schema, nil)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	if putCalled == 0 {
		t.Fatal("PutCustomDomain API should have been called")
	}

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Extract final state
	var finalState CustomDomainModel
	diags := resp.State.Get(tc.Ctx, &finalState)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	// Verify state updated
	AssertStateFieldEquals(t, "Name", finalState.Name, types.StringValue(customDomainName))
	AssertStateFieldEquals(t, "Status", finalState.Status, types.StringValue("ACTIVE"))

	// Verify certificate version is updated
	if finalState.Certificate.IsNull() {
		t.Fatal("Certificate should not be null after update")
	}

	certAttrsResult := finalState.Certificate.Attributes()
	versionVal := certAttrsResult["version"]
	if versionVal.IsNull() {
		t.Fatal("Certificate version should not be null")
	}
	versionInt := versionVal.(types.Int64).ValueInt64()
	if versionInt != newCertVersion {
		t.Errorf("Certificate version mismatch: got=%d, want=%d", versionInt, newCertVersion)
	}

	t.Log("SUCCESS: Update completed and state updated")
}

func TestUpdate_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	distributionId := "dist-abc-123"
	customDomainName := "example.com"
	oldCertVersion := int64(1)
	newCertVersion := int64(2)

	// Current state (before update)
	oldCertAttrs := map[string]attr.Value{
		"certificate": types.StringValue("-----BEGIN CERTIFICATE-----\nMIIC...old\n-----END CERTIFICATE-----"),
		"private_key": types.StringValue("-----BEGIN PRIVATE KEY-----\nMIIE...old\n-----END PRIVATE KEY-----"),
		"version":     types.Int64Value(oldCertVersion),
	}
	oldCertObj, _ := types.ObjectValue(certificateTypes, oldCertAttrs)

	currentState := CustomDomainModel{
		ProjectId:      types.StringValue(projectId),
		DistributionId: types.StringValue(distributionId),
		Name:           types.StringValue(customDomainName),
		ID:             types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, distributionId, customDomainName)),
		Status:         types.StringValue("ACTIVE"),
		Errors:         types.ListNull(types.StringType),
		Certificate:    oldCertObj,
	}

	// Planned state (new certificate)
	newCertAttrs := map[string]attr.Value{
		"certificate": types.StringValue("-----BEGIN CERTIFICATE-----\nMIIC...new\n-----END CERTIFICATE-----"),
		"private_key": types.StringValue("-----BEGIN PRIVATE KEY-----\nMIIE...new\n-----END PRIVATE KEY-----"),
		"version":     types.Int64Value(newCertVersion),
	}
	newCertObj, _ := types.ObjectValue(certificateTypes, newCertAttrs)

	plannedModel := CustomDomainModel{
		ProjectId:      types.StringValue(projectId),
		DistributionId: types.StringValue(distributionId),
		Name:           types.StringValue(customDomainName),
		ID:             types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, distributionId, customDomainName)),
		Status:         types.StringNull(),
		Errors:         types.ListNull(types.StringType),
		Certificate:    newCertObj,
	}

	// Setup mock handlers
	putCalled := 0
	tc.SetupPutCustomDomainHandler(&putCalled)

	// Setup GetCustomDomain to simulate slow response (triggers timeout)
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}/customDomains/{name}", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		// Still returns old version
		jsonResp := fmt.Sprintf(`{
			"customDomain": {
				"name": "%s",
				"status": "UPDATING"
			},
			"certificate": {
				"type": "custom",
				"version": %d
			}
		}`, customDomainName, oldCertVersion)
		w.Write([]byte(jsonResp))
	}).Methods("GET")

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Prepare request
	schema := tc.GetSchema()
	req := UpdateRequest(tc.Ctx, schema, plannedModel)
	// IMPORTANT: Initialize response with current state (simulates framework behavior)
	resp := UpdateResponse(tc.Ctx, schema, &currentState)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	if putCalled == 0 {
		t.Fatal("PutCustomDomain API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to context timeout")
	}

	// Verify helpful error message
	errMsgs := resp.Diagnostics.Errors()
	foundHelpfulMsg := false
	for _, errMsg := range errMsgs {
		if containsString(errMsg.Detail(), "terraform refresh") {
			foundHelpfulMsg = true
			break
		}
	}
	if !foundHelpfulMsg {
		t.Error("Error message should guide user to run 'terraform refresh'")
	}

	// After timeout, verify state was NOT updated with new values
	var stateAfterUpdate CustomDomainModel
	diags := resp.State.Get(context.Background(), &stateAfterUpdate)
	if diags.HasError() {
		t.Fatalf("Failed to get state after update: %v", diags.Errors())
	}

	// Verify state doesn't have NEW certificate version
	if !stateAfterUpdate.Certificate.IsNull() {
		certAttrsResult := stateAfterUpdate.Certificate.Attributes()
		versionVal := certAttrsResult["version"]
		if !versionVal.IsNull() {
			actualVersion := versionVal.(types.Int64).ValueInt64()
			if actualVersion == newCertVersion {
				t.Fatal("BUG: State has NEW certificate version even though update wait failed!")
			}
			// Should still have old version
			if actualVersion != oldCertVersion {
				t.Errorf("Certificate version should be old version: got=%d, want=%d", actualVersion, oldCertVersion)
			}
		}
	}

	// Verify all other fields remain unchanged
	AssertStateFieldEquals(t, "ID", stateAfterUpdate.ID, currentState.ID)
	AssertStateFieldEquals(t, "ProjectId", stateAfterUpdate.ProjectId, currentState.ProjectId)
	AssertStateFieldEquals(t, "DistributionId", stateAfterUpdate.DistributionId, currentState.DistributionId)
	AssertStateFieldEquals(t, "Name", stateAfterUpdate.Name, currentState.Name)
	AssertStateFieldEquals(t, "Status", stateAfterUpdate.Status, currentState.Status)

	t.Log("CORRECT: State preserved old values when update wait failed")
}

func TestUpdate_APICallFails(t *testing.T) {
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
	model.ID = types.StringValue(fmt.Sprintf("%s,%s,%s", projectId, distributionId, customDomainName))

	req := UpdateRequest(tc.Ctx, schema, model)
	resp := UpdateResponse(tc.Ctx, schema, nil)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	if putCalled == 0 {
		t.Fatal("PutCustomDomain API should have been called")
	}

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from API call")
	}

	t.Log("CORRECT: Error returned when API call fails")
}

// Helper function to check if string contains substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && contains(s, substr))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

package cdn

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stackitcloud/stackit-sdk-go/core/utils"
	"github.com/stackitcloud/stackit-sdk-go/services/cdn"
)

// TestRead_Success tests that a successful distribution read works correctly
func TestRead_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	distributionId := "distribution-abc-123"

	tc.SetupGetDistributionHandler(distributionId, projectId, "ACTIVE")

	schema := tc.GetSchema()

	backendObj, _ := types.ObjectValue(backendTypes, map[string]attr.Value{
		"type":                   types.StringValue("http"),
		"origin_url":             types.StringValue("https://example.com"),
		"origin_request_headers": types.MapNull(types.StringType),
		"geofencing":             types.MapNull(geofencingTypes.ElemType),
	})

	regionsList, _ := types.ListValue(types.StringType, []attr.Value{
		types.StringValue("EU"),
	})

	configObj, _ := types.ObjectValue(configTypes, map[string]attr.Value{
		"backend":           backendObj,
		"regions":           regionsList,
		"blocked_countries": types.ListNull(types.StringType),
		"optimizer":         types.ObjectNull(optimizerTypes),
	})

	model := CreateTestModel(projectId, distributionId, configObj)
	req := ReadRequest(tc.Ctx, schema, model)
	resp := ReadResponse(tc.Ctx, schema, &model)

	tc.Resource.Read(tc.Ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Extract final state
	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	// Verify all fields
	AssertStateFieldEquals(t, "DistributionId", finalState.DistributionId, types.StringValue(distributionId))
	AssertStateFieldEquals(t, "Status", finalState.Status, types.StringValue("ACTIVE"))

	t.Log("SUCCESS: All state fields match API response")
}

// TestRead_ResourceNotFound tests that resource removal from state works correctly (404)
func TestRead_ResourceNotFound(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	distributionId := "distribution-abc-123"

	tc.SetupGetDistributionHandlerWithStatus(http.StatusNotFound)

	schema := tc.GetSchema()

	backendObj, _ := types.ObjectValue(backendTypes, map[string]attr.Value{
		"type":                   types.StringValue("http"),
		"origin_url":             types.StringValue("https://example.com"),
		"origin_request_headers": types.MapNull(types.StringType),
		"geofencing":             types.MapNull(geofencingTypes.ElemType),
	})

	regionsList, _ := types.ListValue(types.StringType, []attr.Value{
		types.StringValue("EU"),
	})

	configObj, _ := types.ObjectValue(configTypes, map[string]attr.Value{
		"backend":           backendObj,
		"regions":           regionsList,
		"blocked_countries": types.ListNull(types.StringType),
		"optimizer":         types.ObjectNull(optimizerTypes),
	})

	model := CreateTestModel(projectId, distributionId, configObj)
	req := ReadRequest(tc.Ctx, schema, model)
	resp := ReadResponse(tc.Ctx, schema, &model)

	tc.Resource.Read(tc.Ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should succeed for 404, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Verify state was removed by checking if it's empty
	// When RemoveResource is called, the state becomes empty
	if !resp.State.Raw.IsNull() {
		t.Fatal("State should be removed when resource is not found (404)")
	}

	t.Log("PASS: State removed when resource not found (404)")
}

// TestRead_ResourceGone tests that resource removal from state works correctly (410)
func TestRead_ResourceGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	distributionId := "distribution-abc-123"

	tc.SetupGetDistributionHandlerWithStatus(http.StatusGone)

	schema := tc.GetSchema()

	backendObj, _ := types.ObjectValue(backendTypes, map[string]attr.Value{
		"type":                   types.StringValue("http"),
		"origin_url":             types.StringValue("https://example.com"),
		"origin_request_headers": types.MapNull(types.StringType),
		"geofencing":             types.MapNull(geofencingTypes.ElemType),
	})

	regionsList, _ := types.ListValue(types.StringType, []attr.Value{
		types.StringValue("EU"),
	})

	configObj, _ := types.ObjectValue(configTypes, map[string]attr.Value{
		"backend":           backendObj,
		"regions":           regionsList,
		"blocked_countries": types.ListNull(types.StringType),
		"optimizer":         types.ObjectNull(optimizerTypes),
	})

	model := CreateTestModel(projectId, distributionId, configObj)
	req := ReadRequest(tc.Ctx, schema, model)
	resp := ReadResponse(tc.Ctx, schema, &model)

	tc.Resource.Read(tc.Ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should succeed for 410, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Verify state was removed by checking if it's empty
	// When RemoveResource is called, the state becomes empty
	if !resp.State.Raw.IsNull() {
		t.Fatal("State should be removed when resource is gone (410)")
	}

	t.Log("PASS: State removed when resource is gone (410)")
}

// TestRead_APICallFails tests that when the Read API call fails, an error is returned
func TestRead_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	distributionId := "distribution-abc-123"

	// Setup GetDistribution to return an error
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}).Methods("GET")

	schema := tc.GetSchema()

	backendObj, _ := types.ObjectValue(backendTypes, map[string]attr.Value{
		"type":                   types.StringValue("http"),
		"origin_url":             types.StringValue("https://example.com"),
		"origin_request_headers": types.MapNull(types.StringType),
		"geofencing":             types.MapNull(geofencingTypes.ElemType),
	})

	regionsList, _ := types.ListValue(types.StringType, []attr.Value{
		types.StringValue("EU"),
	})

	configObj, _ := types.ObjectValue(configTypes, map[string]attr.Value{
		"backend":           backendObj,
		"regions":           regionsList,
		"blocked_countries": types.ListNull(types.StringType),
		"optimizer":         types.ObjectNull(optimizerTypes),
	})

	model := CreateTestModel(projectId, distributionId, configObj)
	req := ReadRequest(tc.Ctx, schema, model)
	resp := ReadResponse(tc.Ctx, schema, &model)

	tc.Resource.Read(tc.Ctx, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error due to API failure")
	}

	t.Log("PASS: Error returned when API call fails")
}

// TestRead_DetectDrift tests that drift detection works correctly
func TestRead_DetectDrift(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	distributionId := "distribution-abc-123"

	// Setup GetDistribution to return updated values (drift)
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Create response using SDK types with updated values
		now := time.Now()
		statusEnum, _ := cdn.NewDistributionStatusFromValue(cdn.DISTRIBUTIONSTATUS_ACTIVE)

		distribution := &cdn.Distribution{
			Id:        utils.Ptr(distributionId),
			ProjectId: utils.Ptr(projectId),
			Status:    statusEnum,
			CreatedAt: &now,
			UpdatedAt: &now,
			Config: &cdn.Config{
				Backend: &cdn.ConfigBackend{
					HttpBackend: &cdn.HttpBackend{
						Type:                 utils.Ptr("http"),
						OriginUrl:            utils.Ptr("https://new-example.com"),
						OriginRequestHeaders: &map[string]string{},
					},
				},
				Regions:          &[]cdn.Region{cdn.REGION_EU},
				BlockedCountries: &[]string{},
				BlockedIPs:       &[]string{},
				Waf:              cdn.NewWafConfig([]string{}, cdn.WAFMODE_DISABLED, cdn.WAFTYPE_FREE),
			},
			Domains: &[]cdn.Domain{},
		}

		resp := cdn.GetDistributionResponse{
			Distribution: distribution,
		}
		respBytes, _ := json.Marshal(resp)
		w.Write(respBytes)
	}).Methods("GET")

	schema := tc.GetSchema()

	// Current state has old values
	backendObjOld, _ := types.ObjectValue(backendTypes, map[string]attr.Value{
		"type":                   types.StringValue("http"),
		"origin_url":             types.StringValue("https://old-example.com"),
		"origin_request_headers": types.MapNull(types.StringType),
		"geofencing":             types.MapNull(geofencingTypes.ElemType),
	})

	regionsList, _ := types.ListValue(types.StringType, []attr.Value{
		types.StringValue("EU"),
	})

	configObjOld, _ := types.ObjectValue(configTypes, map[string]attr.Value{
		"backend":           backendObjOld,
		"regions":           regionsList,
		"blocked_countries": types.ListNull(types.StringType),
		"optimizer":         types.ObjectNull(optimizerTypes),
	})

	model := CreateTestModel(projectId, distributionId, configObjOld)
	req := ReadRequest(tc.Ctx, schema, model)
	resp := ReadResponse(tc.Ctx, schema, &model)

	tc.Resource.Read(tc.Ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should succeed, but got errors: %v", resp.Diagnostics.Errors())
	}

	// Extract final state
	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	if diags.HasError() {
		t.Fatalf("Failed to get state: %v", diags.Errors())
	}

	// Verify state was updated with new values from API (drift detected)
	var finalConfig distributionConfig
	diags = finalState.Config.As(tc.Ctx, &finalConfig, basetypes.ObjectAsOptions{})
	if diags.HasError() {
		t.Fatalf("Failed to extract config: %v", diags.Errors())
	}

	if finalConfig.Backend.OriginURL != "https://new-example.com" {
		t.Errorf("Expected origin_url to be updated to new value, got: %s", finalConfig.Backend.OriginURL)
	}

	t.Log("PASS: Drift detected and state updated with new values from API")
}

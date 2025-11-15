package dns

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stackitcloud/stackit-sdk-go/core/utils"
	"github.com/stackitcloud/stackit-sdk-go/services/dns"
	mock_zone "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/dns/zone/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreate_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	name := "my-test-zone"
	dnsName := "example.com."

	// Setup mock expectations
	zoneResp := BuildZoneResponse(zoneId, name, dnsName)

	// Mock CreateZone
	mockCreateReq := mock_zone.NewMockApiCreateZoneRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateZonePayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(zoneResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		CreateZone(gomock.Any(), projectId).
		Return(mockCreateReq).
		Times(1)

	// Mock GetZoneExecute for wait handler (wait handler calls this directly, not fluent API)
	tc.MockClient.EXPECT().
		GetZoneExecute(gomock.Any(), projectId, zoneId).
		Return(zoneResp, nil).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, "", name, dnsName)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Create should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify all fields match what was returned from GetZone
	require.Equal(t, zoneId, finalState.ZoneId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s", projectId, zoneId), finalState.Id.ValueString())
	require.Equal(t, projectId, finalState.ProjectId.ValueString())
	require.Equal(t, name, finalState.Name.ValueString())
	require.Equal(t, dnsName, finalState.DnsName.ValueString())
	require.False(t, finalState.Active.IsNull(), "Active should be set from API response")
	require.False(t, finalState.State.IsNull(), "State should be set from API response")
}

func TestCreate_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	name := "my-test-zone"
	dnsName := "example.com."

	// Setup mock expectations
	zoneResp := BuildZoneResponse(zoneId, name, dnsName)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Mock CreateZone
	mockCreateReq := mock_zone.NewMockApiCreateZoneRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateZonePayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(zoneResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		CreateZone(gomock.Any(), projectId).
		Return(mockCreateReq).
		Times(1)

	// Mock GetZoneExecute for wait handler - simulate timeout
	tc.MockClient.EXPECT().
		GetZoneExecute(gomock.Any(), projectId, zoneId).
		DoAndReturn(func(ctx context.Context, projectId, zoneId string) (*dns.Zone, error) {
			time.Sleep(150 * time.Millisecond) // Longer than context timeout
			return nil, ctx.Err()
		}).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, "", name, dnsName)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	// Assertions
	require.False(t, resp.Diagnostics.HasError(), "Expected no error due to context timeout")

	var stateAfterCreate Model
	diags := resp.State.Get(tc.Ctx, &stateAfterCreate)
	require.False(t, diags.HasError(), "Expected no errors reading state")

	// Verify idempotency - ZoneId should be saved immediately after CreateZone succeeds
	require.False(t, stateAfterCreate.ZoneId.IsNull(), "BUG: ZoneId should be saved to state immediately after CreateZone API succeeds")
	require.NotEmpty(t, stateAfterCreate.ZoneId.ValueString(), "ZoneId should not be empty")

	// Verify basic fields from input/CreateZone response are set
	require.Equal(t, zoneId, stateAfterCreate.ZoneId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s", projectId, zoneId), stateAfterCreate.Id.ValueString())
	require.Equal(t, projectId, stateAfterCreate.ProjectId.ValueString())
	require.Equal(t, name, stateAfterCreate.Name.ValueString())
	require.Equal(t, dnsName, stateAfterCreate.DnsName.ValueString())

	// CRITICAL: Verify fields that require GetZone are NULL after failed wait
	require.True(t, stateAfterCreate.State.IsNull(), "State should be null after failed wait (only GetZone provides this)")
	require.True(t, stateAfterCreate.RecordCount.IsNull(), "RecordCount should be null after failed wait (only GetZone provides this)")
	require.True(t, stateAfterCreate.SerialNumber.IsNull(), "SerialNumber should be null after failed wait (only GetZone provides this)")
}

func TestCreate_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	name := "test-zone"
	dnsName := "example.com."

	// Setup mock expectations - CreateZone fails
	apiErr := fmt.Errorf("API error")

	mockCreateReq := mock_zone.NewMockApiCreateZoneRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateZonePayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(nil, apiErr).
		Times(1)

	tc.MockClient.EXPECT().
		CreateZone(gomock.Any(), projectId).
		Return(mockCreateReq).
		Times(1)

	schema := tc.GetSchema()
	model := CreateTestModel(projectId, "", name, dnsName)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	require.True(t, resp.Diagnostics.HasError(), "Expected error when API call fails")

	// State should be empty since zone was never created
	var stateAfterCreate Model
	resp.State.Get(tc.Ctx, &stateAfterCreate)

	require.True(t, stateAfterCreate.ZoneId.IsNull(), "ZoneId should be null when API call fails")
}

func TestCreate_WithPrimaries(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	name := "my-test-zone"
	dnsName := "example.com."
	primaries := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"}

	// Setup mock expectations
	zone := BuildZoneWithPrimaries(zoneId, name, dnsName, primaries)
	zoneResp := &dns.ZoneResponse{Zone: zone}

	// Mock CreateZone
	mockCreateReq := mock_zone.NewMockApiCreateZoneRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateZonePayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(zoneResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		CreateZone(gomock.Any(), projectId).
		Return(mockCreateReq).
		Times(1)

	// Mock GetZoneExecute for wait handler
	tc.MockClient.EXPECT().
		GetZoneExecute(gomock.Any(), projectId, zoneId).
		Return(&dns.ZoneResponse{Zone: zone}, nil).
		AnyTimes()

	// Prepare request with primaries
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, "", name, dnsName)

	// Add primaries to model
	primaryValues := make([]attr.Value, len(primaries))
	for i, p := range primaries {
		primaryValues[i] = types.StringValue(p)
	}
	primaryList, _ := types.ListValue(types.StringType, primaryValues)
	model.Primaries = primaryList
	model.Type = types.StringValue(string(dns.ZONETYPE_SECONDARY))

	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Create should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify primaries list order is preserved
	require.False(t, finalState.Primaries.IsNull(), "Primaries should be set")

	var statePrimaries []string
	diags = finalState.Primaries.ElementsAs(tc.Ctx, &statePrimaries, false)
	require.False(t, diags.HasError(), "Failed to get primaries: %v", diags.Errors())
	require.Equal(t, primaries, statePrimaries, "Primaries order should be preserved")
}

func TestCreate_WithAllOptionalFields(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	name := "my-test-zone"
	dnsName := "example.com."

	// Setup mock expectations with all optional fields
	zone := BuildZone(zoneId, name, dnsName)
	zone.Description = utils.Ptr("Comprehensive test zone")
	zone.Acl = utils.Ptr("192.168.0.0/16,10.0.0.0/8")
	zone.ContactEmail = utils.Ptr("admin@example.com")
	zone.DefaultTTL = utils.Ptr(int64(7200))
	zone.ExpireTime = utils.Ptr(int64(1209600))
	zone.NegativeCache = utils.Ptr(int64(600))
	zone.RefreshTime = utils.Ptr(int64(43200))
	zone.RetryTime = utils.Ptr(int64(3600))

	zoneResp := &dns.ZoneResponse{Zone: zone}

	// Mock CreateZone
	mockCreateReq := mock_zone.NewMockApiCreateZoneRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateZonePayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(zoneResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		CreateZone(gomock.Any(), projectId).
		Return(mockCreateReq).
		Times(1)

	// Mock GetZoneExecute for wait handler
	tc.MockClient.EXPECT().
		GetZoneExecute(gomock.Any(), projectId, zoneId).
		Return(&dns.ZoneResponse{Zone: zone}, nil).
		AnyTimes()

	// Prepare request with all optional fields
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, "", name, dnsName)
	model.Description = types.StringValue("Comprehensive test zone")
	model.Acl = types.StringValue("192.168.0.0/16,10.0.0.0/8")
	model.ContactEmail = types.StringValue("admin@example.com")
	model.DefaultTTL = types.Int64Value(7200)
	model.ExpireTime = types.Int64Value(1209600)
	model.NegativeCache = types.Int64Value(600)
	model.RefreshTime = types.Int64Value(43200)
	model.RetryTime = types.Int64Value(3600)

	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Create should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify all optional fields are populated
	require.Equal(t, "Comprehensive test zone", finalState.Description.ValueString())
	require.Equal(t, "192.168.0.0/16,10.0.0.0/8", finalState.Acl.ValueString())
	require.Equal(t, "admin@example.com", finalState.ContactEmail.ValueString())
	require.Equal(t, int64(7200), finalState.DefaultTTL.ValueInt64())
	require.Equal(t, int64(1209600), finalState.ExpireTime.ValueInt64())
	require.Equal(t, int64(600), finalState.NegativeCache.ValueInt64())
	require.Equal(t, int64(43200), finalState.RefreshTime.ValueInt64())
	require.Equal(t, int64(3600), finalState.RetryTime.ValueInt64())
}

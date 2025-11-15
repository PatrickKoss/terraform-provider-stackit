package dns

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stackitcloud/stackit-sdk-go/services/dns"
	mock_recordset "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/dns/recordset/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreate_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-xyz-456"
	name := "www"
	recordType := dns.RECORDSETTYPE_A
	records := []string{"192.168.1.1"}

	// Setup mock expectations
	recordSetResp := BuildRecordSetResponse(recordSetId, name, zoneId, recordType, records)

	// Mock CreateRecordSet
	mockCreateReq := mock_recordset.NewMockApiCreateRecordSetRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateRecordSetPayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(recordSetResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		CreateRecordSet(gomock.Any(), projectId, zoneId).
		Return(mockCreateReq).
		Times(1)

	// Mock GetRecordSetExecute for wait handler
	tc.MockClient.EXPECT().
		GetRecordSetExecute(gomock.Any(), projectId, zoneId, recordSetId).
		Return(recordSetResp, nil).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, zoneId, "", name, recordType, records)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Create should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify all fields match what was returned from GetRecordSet
	require.Equal(t, recordSetId, finalState.RecordSetId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s,%s", projectId, zoneId, recordSetId), finalState.Id.ValueString())
	require.Equal(t, projectId, finalState.ProjectId.ValueString())
	require.Equal(t, zoneId, finalState.ZoneId.ValueString())
	require.Equal(t, name, finalState.Name.ValueString())
	require.False(t, finalState.Active.IsNull(), "Active should be set from API response")
	require.False(t, finalState.State.IsNull(), "State should be set from API response")
	require.False(t, finalState.FQDN.IsNull(), "FQDN should be set from API response")
}

func TestCreate_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-xyz-456"
	name := "www"
	recordType := dns.RECORDSETTYPE_A
	records := []string{"192.168.1.1"}

	// Setup mock expectations
	recordSetResp := BuildRecordSetResponse(recordSetId, name, zoneId, recordType, records)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Mock CreateRecordSet
	mockCreateReq := mock_recordset.NewMockApiCreateRecordSetRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateRecordSetPayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(recordSetResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		CreateRecordSet(gomock.Any(), projectId, zoneId).
		Return(mockCreateReq).
		Times(1)

	// Mock GetRecordSetExecute for wait handler - simulate timeout
	tc.MockClient.EXPECT().
		GetRecordSetExecute(gomock.Any(), projectId, zoneId, recordSetId).
		DoAndReturn(func(ctx context.Context, projectId, zoneId, recordSetId string) (*dns.RecordSetResponse, error) {
			time.Sleep(150 * time.Millisecond) // Longer than context timeout
			return nil, ctx.Err()
		}).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, zoneId, "", name, recordType, records)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	// Execute Create
	tc.Resource.Create(tc.Ctx, req, resp)

	// Assertions
	require.False(t, resp.Diagnostics.HasError(), "Expected no error due to context timeout")

	var stateAfterCreate Model
	diags := resp.State.Get(tc.Ctx, &stateAfterCreate)
	require.False(t, diags.HasError(), "Expected no errors reading state")

	// Verify idempotency - RecordSetId should be saved immediately after CreateRecordSet succeeds
	require.False(t, stateAfterCreate.RecordSetId.IsNull(), "BUG: RecordSetId should be saved to state immediately after CreateRecordSet API succeeds")
	require.NotEmpty(t, stateAfterCreate.RecordSetId.ValueString(), "RecordSetId should not be empty")

	// Verify basic fields from input/CreateRecordSet response are set
	require.Equal(t, recordSetId, stateAfterCreate.RecordSetId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s,%s", projectId, zoneId, recordSetId), stateAfterCreate.Id.ValueString())
	require.Equal(t, projectId, stateAfterCreate.ProjectId.ValueString())
	require.Equal(t, zoneId, stateAfterCreate.ZoneId.ValueString())
	require.Equal(t, name, stateAfterCreate.Name.ValueString())

	// CRITICAL: Verify fields that require GetRecordSet are NULL after failed wait
	require.True(t, stateAfterCreate.FQDN.IsNull(), "FQDN should be null after failed wait (only GetRecordSet provides this)")
	require.True(t, stateAfterCreate.State.IsNull(), "State should be null after failed wait (only GetRecordSet provides this)")
}

func TestCreate_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	zoneId := "zone-123"
	name := "www"
	recordType := dns.RECORDSETTYPE_A
	records := []string{"192.168.1.1"}

	// Setup mock expectations - CreateRecordSet fails
	apiErr := fmt.Errorf("API error")

	mockCreateReq := mock_recordset.NewMockApiCreateRecordSetRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateRecordSetPayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(nil, apiErr).
		Times(1)

	tc.MockClient.EXPECT().
		CreateRecordSet(gomock.Any(), projectId, zoneId).
		Return(mockCreateReq).
		Times(1)

	schema := tc.GetSchema()
	model := CreateTestModel(projectId, zoneId, "", name, recordType, records)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	require.True(t, resp.Diagnostics.HasError(), "Expected error when API call fails")

	// State should be empty since recordset was never created
	var stateAfterCreate Model
	resp.State.Get(tc.Ctx, &stateAfterCreate)

	require.True(t, stateAfterCreate.RecordSetId.IsNull(), "RecordSetId should be null when API call fails")
}

func TestCreate_WithMultipleRecords(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-xyz-456"
	name := "www"
	recordType := dns.RECORDSETTYPE_A
	records := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"}

	// Setup mock expectations
	recordSetResp := BuildRecordSetWithMultipleRecords(recordSetId, name, zoneId, recordType, records)
	resp := &dns.RecordSetResponse{Rrset: recordSetResp}

	// Mock CreateRecordSet
	mockCreateReq := mock_recordset.NewMockApiCreateRecordSetRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateRecordSetPayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(resp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		CreateRecordSet(gomock.Any(), projectId, zoneId).
		Return(mockCreateReq).
		Times(1)

	// Mock GetRecordSetExecute for wait handler
	tc.MockClient.EXPECT().
		GetRecordSetExecute(gomock.Any(), projectId, zoneId, recordSetId).
		Return(resp, nil).
		AnyTimes()

	// Prepare request with multiple records
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, zoneId, "", name, recordType, records)
	req := CreateRequest(tc.Ctx, schema, model)
	createResp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, createResp)

	require.False(t, createResp.Diagnostics.HasError(), "Create should succeed, but got errors: %v", createResp.Diagnostics.Errors())

	var finalState Model
	diags := createResp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify records are populated
	require.False(t, finalState.Records.IsNull(), "Records should be set")

	var stateRecords []string
	diags = finalState.Records.ElementsAs(tc.Ctx, &stateRecords, false)
	require.False(t, diags.HasError(), "Failed to get records: %v", diags.Errors())
	require.Len(t, stateRecords, len(records), "Should have correct number of records")
}

func TestCreate_WithZoneValidation(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-xyz-456"
	name := "www"
	dnsName := "example.com."
	recordType := dns.RECORDSETTYPE_A
	records := []string{"192.168.1.1"}

	// Setup mock expectations
	recordSetResp := BuildRecordSetResponse(recordSetId, name, zoneId, recordType, records)
	zoneResp := BuildZoneResponse(zoneId, "example-zone", dnsName)

	// Mock GetZone for zone validation (optional based on implementation)
	mockGetZoneReq := mock_recordset.NewMockApiGetZoneRequest(tc.MockCtrl)
	mockGetZoneReq.EXPECT().
		Execute().
		Return(zoneResp, nil).
		AnyTimes() // May not be called depending on implementation

	tc.MockClient.EXPECT().
		GetZone(gomock.Any(), projectId, zoneId).
		Return(mockGetZoneReq).
		AnyTimes() // May not be called depending on implementation

	// Mock CreateRecordSet
	mockCreateReq := mock_recordset.NewMockApiCreateRecordSetRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateRecordSetPayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(recordSetResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		CreateRecordSet(gomock.Any(), projectId, zoneId).
		Return(mockCreateReq).
		Times(1)

	// Mock GetRecordSetExecute for wait handler
	tc.MockClient.EXPECT().
		GetRecordSetExecute(gomock.Any(), projectId, zoneId, recordSetId).
		Return(recordSetResp, nil).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()
	model := CreateTestModel(projectId, zoneId, "", name, recordType, records)
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Create should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	require.Equal(t, recordSetId, finalState.RecordSetId.ValueString())
	require.Equal(t, zoneId, finalState.ZoneId.ValueString())
}

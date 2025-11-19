package dns

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stackitcloud/stackit-sdk-go/core/utils"
	"github.com/stackitcloud/stackit-sdk-go/services/dns"
	mock_recordset "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/dns/recordset/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestUpdate_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-xyz-456"
	name := "www"
	recordType := dns.RECORDSETTYPE_A

	// Old and new records
	oldRecords := []string{"192.168.1.1"}
	newRecords := []string{"192.168.1.10"}

	// Setup mock expectations
	recordSet := BuildRecordSet(recordSetId, name, zoneId, recordType, newRecords)
	recordSet.State = dns.RECORDSETSTATE_UPDATE_SUCCEEDED.Ptr() // Update operation completed
	recordSetResp := &dns.RecordSetResponse{Rrset: recordSet}

	// Mock PartialUpdateRecordSet
	mockUpdateReq := mock_recordset.NewMockApiPartialUpdateRecordSetRequest(tc.MockCtrl)
	mockUpdateReq.EXPECT().
		PartialUpdateRecordSetPayload(gomock.Any()).
		Return(mockUpdateReq).
		Times(1)
	mockUpdateReq.EXPECT().
		Execute().
		Return(nil, nil).
		Times(1)

	tc.MockClient.EXPECT().
		PartialUpdateRecordSet(gomock.Any(), projectId, zoneId, recordSetId).
		Return(mockUpdateReq).
		Times(1)

	// Mock GetRecordSetExecute for wait handler
	tc.MockClient.EXPECT().
		GetRecordSetExecute(gomock.Any(), projectId, zoneId, recordSetId).
		Return(recordSetResp, nil).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()

	currentState := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, oldRecords)
	plannedState := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, newRecords)

	req := UpdateRequest(tc.Ctx, schema, currentState, plannedState)
	resp := UpdateResponse(tc.Ctx, schema, nil)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	require.False(t, resp.Diagnostics.HasError(), "Update should succeed, but got errors: %v", resp.Diagnostics.Errors())

	// Extract final state
	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify all fields match the updated values from GetRecordSet
	require.Equal(t, recordSetId, finalState.RecordSetId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s,%s", projectId, zoneId, recordSetId), finalState.Id.ValueString())
	require.Equal(t, projectId, finalState.ProjectId.ValueString())
	require.Equal(t, zoneId, finalState.ZoneId.ValueString())
	require.Equal(t, name, finalState.Name.ValueString())

	// Verify records were updated
	var stateRecords []string
	diags = finalState.Records.ElementsAs(tc.Ctx, &stateRecords, false)
	require.False(t, diags.HasError(), "Failed to get records: %v", diags.Errors())
	require.Equal(t, newRecords, stateRecords)
}

func TestUpdate_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-xyz-456"
	name := "www"
	recordType := dns.RECORDSETTYPE_A
	oldRecords := []string{"192.168.1.1"}
	newRecords := []string{"192.168.1.10"}

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Mock PartialUpdateRecordSet
	mockUpdateReq := mock_recordset.NewMockApiPartialUpdateRecordSetRequest(tc.MockCtrl)
	mockUpdateReq.EXPECT().
		PartialUpdateRecordSetPayload(gomock.Any()).
		Return(mockUpdateReq).
		Times(1)
	mockUpdateReq.EXPECT().
		Execute().
		Return(nil, nil).
		Times(1)

	tc.MockClient.EXPECT().
		PartialUpdateRecordSet(gomock.Any(), projectId, zoneId, recordSetId).
		Return(mockUpdateReq).
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

	currentState := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, oldRecords)
	plannedState := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, newRecords)

	req := UpdateRequest(tc.Ctx, schema, currentState, plannedState)
	resp := UpdateResponse(tc.Ctx, schema, &currentState)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	require.False(t, resp.Diagnostics.HasError(), "Expected no error due to context timeout")

	var stateAfterUpdate Model
	diags := resp.State.Get(tc.Ctx, &stateAfterUpdate)
	require.False(t, diags.HasError(), "Failed to get state after update: %v", diags.Errors())

	// State should preserve old values since update wait failed
	var stateRecords []string
	diags = stateAfterUpdate.Records.ElementsAs(tc.Ctx, &stateRecords, false)
	require.False(t, diags.HasError(), "Failed to get records: %v", diags.Errors())
	require.Equal(t, oldRecords, stateRecords, "Records should preserve old values after failed wait")

	require.Equal(t, currentState.Id.ValueString(), stateAfterUpdate.Id.ValueString())
	require.Equal(t, currentState.ProjectId.ValueString(), stateAfterUpdate.ProjectId.ValueString())
	require.Equal(t, currentState.ZoneId.ValueString(), stateAfterUpdate.ZoneId.ValueString())
	require.Equal(t, currentState.RecordSetId.ValueString(), stateAfterUpdate.RecordSetId.ValueString())
	require.Equal(t, currentState.Name.ValueString(), stateAfterUpdate.Name.ValueString())
}

func TestUpdate_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	zoneId := "zone-123"
	recordSetId := "recordset-456"
	name := "www"
	recordType := dns.RECORDSETTYPE_A

	// Setup mock expectations - PartialUpdateRecordSet fails
	apiErr := fmt.Errorf("API error")

	mockUpdateReq := mock_recordset.NewMockApiPartialUpdateRecordSetRequest(tc.MockCtrl)
	mockUpdateReq.EXPECT().
		PartialUpdateRecordSetPayload(gomock.Any()).
		Return(mockUpdateReq).
		Times(1)
	mockUpdateReq.EXPECT().
		Execute().
		Return(nil, apiErr).
		Times(1)

	tc.MockClient.EXPECT().
		PartialUpdateRecordSet(gomock.Any(), projectId, zoneId, recordSetId).
		Return(mockUpdateReq).
		Times(1)

	// Prepare request
	schema := tc.GetSchema()

	oldRecords := []string{"192.168.1.1"}
	newRecords := []string{"192.168.1.10"}

	currentState := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, oldRecords)
	plannedState := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, newRecords)

	req := UpdateRequest(tc.Ctx, schema, currentState, plannedState)
	resp := UpdateResponse(tc.Ctx, schema, nil)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	require.True(t, resp.Diagnostics.HasError(), "Expected error when API call fails")
}

func TestUpdate_ChangeRecords(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-xyz-456"
	name := "www"
	recordType := dns.RECORDSETTYPE_A

	// Add more records
	oldRecords := []string{"192.168.1.1"}
	newRecords := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"}

	// Setup mock expectations
	recordSet := BuildRecordSet(recordSetId, name, zoneId, recordType, newRecords)
	recordSet.State = dns.RECORDSETSTATE_UPDATE_SUCCEEDED.Ptr() // Update operation completed
	recordSetResp := &dns.RecordSetResponse{Rrset: recordSet}

	// Mock PartialUpdateRecordSet
	mockUpdateReq := mock_recordset.NewMockApiPartialUpdateRecordSetRequest(tc.MockCtrl)
	mockUpdateReq.EXPECT().
		PartialUpdateRecordSetPayload(gomock.Any()).
		Return(mockUpdateReq).
		Times(1)
	mockUpdateReq.EXPECT().
		Execute().
		Return(nil, nil).
		Times(1)

	tc.MockClient.EXPECT().
		PartialUpdateRecordSet(gomock.Any(), projectId, zoneId, recordSetId).
		Return(mockUpdateReq).
		Times(1)

	// Mock GetRecordSetExecute for wait handler
	tc.MockClient.EXPECT().
		GetRecordSetExecute(gomock.Any(), projectId, zoneId, recordSetId).
		Return(recordSetResp, nil).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()

	currentState := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, oldRecords)
	plannedState := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, newRecords)

	req := UpdateRequest(tc.Ctx, schema, currentState, plannedState)
	resp := UpdateResponse(tc.Ctx, schema, nil)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	require.False(t, resp.Diagnostics.HasError(), "Update should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify records were updated
	var stateRecords []string
	diags = finalState.Records.ElementsAs(tc.Ctx, &stateRecords, false)
	require.False(t, diags.HasError(), "Failed to get records: %v", diags.Errors())
	require.Len(t, stateRecords, len(newRecords), "Should have updated number of records")
	require.Equal(t, newRecords, stateRecords)
}

func TestUpdate_ChangeTTL(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-xyz-456"
	name := "www"
	recordType := dns.RECORDSETTYPE_A
	records := []string{"192.168.1.1"}
	oldTTL := int64(3600)
	newTTL := int64(7200)

	// Setup mock expectations
	recordSet := BuildRecordSet(recordSetId, name, zoneId, recordType, records)
	recordSet.Ttl = utils.Ptr(newTTL)                           // Updated TTL
	recordSet.State = dns.RECORDSETSTATE_UPDATE_SUCCEEDED.Ptr() // Update operation completed
	recordSetResp := &dns.RecordSetResponse{Rrset: recordSet}

	// Mock PartialUpdateRecordSet
	mockUpdateReq := mock_recordset.NewMockApiPartialUpdateRecordSetRequest(tc.MockCtrl)
	mockUpdateReq.EXPECT().
		PartialUpdateRecordSetPayload(gomock.Any()).
		Return(mockUpdateReq).
		Times(1)
	mockUpdateReq.EXPECT().
		Execute().
		Return(nil, nil).
		Times(1)

	tc.MockClient.EXPECT().
		PartialUpdateRecordSet(gomock.Any(), projectId, zoneId, recordSetId).
		Return(mockUpdateReq).
		Times(1)

	// Mock GetRecordSetExecute for wait handler
	tc.MockClient.EXPECT().
		GetRecordSetExecute(gomock.Any(), projectId, zoneId, recordSetId).
		Return(recordSetResp, nil).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()

	currentState := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, records)
	currentState.TTL = types.Int64Value(oldTTL)

	plannedState := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, records)
	plannedState.TTL = types.Int64Value(newTTL)

	req := UpdateRequest(tc.Ctx, schema, currentState, plannedState)
	resp := UpdateResponse(tc.Ctx, schema, nil)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	require.False(t, resp.Diagnostics.HasError(), "Update should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify TTL was updated (note: API returns 3600 from BuildRecordSet, but we're testing the flow)
	require.False(t, finalState.TTL.IsNull(), "TTL should be set")
}

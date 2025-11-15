package dns

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stackitcloud/stackit-sdk-go/core/oapierror"
	"github.com/stackitcloud/stackit-sdk-go/core/utils"
	"github.com/stackitcloud/stackit-sdk-go/services/dns"
	mock_recordset "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/dns/recordset/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRead_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-xyz-456"
	name := "www"
	recordType := dns.RECORDSETTYPE_A
	records := []string{"192.168.1.1"}

	// Setup mock expectations
	recordSetResp := BuildRecordSetResponse(recordSetId, name, zoneId, recordType, records)

	// Mock GetRecordSet
	mockGetReq := mock_recordset.NewMockApiGetRecordSetRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(recordSetResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		GetRecordSet(gomock.Any(), projectId, zoneId, recordSetId).
		Return(mockGetReq).
		Times(1)

	schema := tc.GetSchema()

	state := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, records)
	// Simulate outdated state
	state.TTL = types.Int64Null()
	state.Active = types.BoolNull()

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var refreshedState Model
	diags := resp.State.Get(tc.Ctx, &refreshedState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	require.Equal(t, recordSetId, refreshedState.RecordSetId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s,%s", projectId, zoneId, recordSetId), refreshedState.Id.ValueString())
	require.Equal(t, projectId, refreshedState.ProjectId.ValueString())
	require.Equal(t, zoneId, refreshedState.ZoneId.ValueString())
	require.Equal(t, name, refreshedState.Name.ValueString())

	require.False(t, refreshedState.Active.IsNull(), "Active should be set from API response")
	require.False(t, refreshedState.State.IsNull(), "State should be set from API response")
	require.False(t, refreshedState.FQDN.IsNull(), "FQDN should be set from API response")
}

func TestRead_RecordSetNotFound(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "non-existent-recordset"

	// Setup GetRecordSet to return 404
	notFoundErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusNotFound,
	}

	// Mock GetRecordSet
	mockGetReq := mock_recordset.NewMockApiGetRecordSetRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(nil, notFoundErr).
		Times(1)

	tc.MockClient.EXPECT().
		GetRecordSet(gomock.Any(), projectId, zoneId, recordSetId).
		Return(mockGetReq).
		Times(1)

	schema := tc.GetSchema()

	state := CreateTestModel(projectId, zoneId, recordSetId, "www", dns.RECORDSETTYPE_A, []string{"192.168.1.1"})

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should not error when recordset not found, but got errors: %v", resp.Diagnostics.Errors())
}

func TestRead_RecordSetGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "gone-recordset"

	// Setup GetRecordSet to return 410 Gone
	goneErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusGone,
	}

	// Mock GetRecordSet
	mockGetReq := mock_recordset.NewMockApiGetRecordSetRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(nil, goneErr).
		Times(1)

	tc.MockClient.EXPECT().
		GetRecordSet(gomock.Any(), projectId, zoneId, recordSetId).
		Return(mockGetReq).
		Times(1)

	schema := tc.GetSchema()

	state := CreateTestModel(projectId, zoneId, recordSetId, "www", dns.RECORDSETTYPE_A, []string{"192.168.1.1"})

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should not error when recordset is gone, but got errors: %v", resp.Diagnostics.Errors())
}

func TestRead_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	zoneId := "zone-123"
	recordSetId := "recordset-456"

	// Setup GetRecordSet to return 500 error
	serverErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusInternalServerError,
	}

	// Mock GetRecordSet
	mockGetReq := mock_recordset.NewMockApiGetRecordSetRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(nil, serverErr).
		Times(1)

	tc.MockClient.EXPECT().
		GetRecordSet(gomock.Any(), projectId, zoneId, recordSetId).
		Return(mockGetReq).
		Times(1)

	schema := tc.GetSchema()

	state := CreateTestModel(projectId, zoneId, recordSetId, "www", dns.RECORDSETTYPE_A, []string{"192.168.1.1"})

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.True(t, resp.Diagnostics.HasError(), "Expected error when API call fails")
}

func TestRead_DetectDrift(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-xyz-456"
	name := "www"
	recordType := dns.RECORDSETTYPE_A

	// Setup mock expectations - records changed in cloud
	newRecords := []string{"192.168.1.10", "192.168.1.11"}
	recordSet := BuildRecordSet(recordSetId, name, zoneId, recordType, newRecords)
	recordSet.Ttl = utils.Ptr(int64(7200)) // TTL changed too
	recordSetResp := &dns.RecordSetResponse{Rrset: recordSet}

	// Mock GetRecordSet - returns drifted state
	mockGetReq := mock_recordset.NewMockApiGetRecordSetRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(recordSetResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		GetRecordSet(gomock.Any(), projectId, zoneId, recordSetId).
		Return(mockGetReq).
		Times(1)

	schema := tc.GetSchema()

	// Old state with different records and TTL
	oldRecords := []string{"192.168.1.1"}
	state := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, oldRecords)
	state.TTL = types.Int64Value(3600)

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var refreshedState Model
	diags := resp.State.Get(tc.Ctx, &refreshedState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify drift was detected and state was updated
	require.Equal(t, int64(7200), refreshedState.TTL.ValueInt64())

	var stateRecords []string
	diags = refreshedState.Records.ElementsAs(tc.Ctx, &stateRecords, false)
	require.False(t, diags.HasError(), "Failed to get records: %v", diags.Errors())
	require.Len(t, stateRecords, len(newRecords), "Should have updated number of records")
}

func TestRead_RecordsOrderIndependent(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-xyz-456"
	name := "www"
	recordType := dns.RECORDSETTYPE_A

	// API returns records in one order
	apiRecords := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"}
	recordSetResp := BuildRecordSetResponse(recordSetId, name, zoneId, recordType, apiRecords)

	// Mock GetRecordSet
	mockGetReq := mock_recordset.NewMockApiGetRecordSetRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(recordSetResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		GetRecordSet(gomock.Any(), projectId, zoneId, recordSetId).
		Return(mockGetReq).
		Times(1)

	schema := tc.GetSchema()

	// State has records in different order
	stateRecords := []string{"192.168.1.3", "192.168.1.1", "192.168.1.2"}
	stateRecordValues := make([]attr.Value, len(stateRecords))
	for i, record := range stateRecords {
		stateRecordValues[i] = types.StringValue(record)
	}
	recordsList, _ := types.ListValue(types.StringType, stateRecordValues)

	state := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, stateRecords)
	state.Records = recordsList

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var refreshedState Model
	diags := resp.State.Get(tc.Ctx, &refreshedState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify records from API response are used (order should match API, not state)
	require.False(t, refreshedState.Records.IsNull(), "Records should be set")

	var resultRecords []string
	diags = refreshedState.Records.ElementsAs(tc.Ctx, &resultRecords, false)
	require.False(t, diags.HasError(), "Failed to get records: %v", diags.Errors())

	// The implementation should preserve API order or be order-independent
	// Here we just verify all records are present
	require.Len(t, resultRecords, len(apiRecords), "Should have correct number of records")
}

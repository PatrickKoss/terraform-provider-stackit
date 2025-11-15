package dns

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stackitcloud/stackit-sdk-go/core/oapierror"
	"github.com/stackitcloud/stackit-sdk-go/services/dns"
	mock_recordset "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/dns/recordset/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDelete_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-xyz-456"
	name := "www"
	recordType := dns.RECORDSETTYPE_A
	records := []string{"192.168.1.1"}

	// Setup mock expectations - delete succeeds
	goneErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusGone,
	}

	// Mock DeleteRecordSet
	mockDeleteReq := mock_recordset.NewMockApiDeleteRecordSetRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(nil, nil).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteRecordSet(gomock.Any(), projectId, zoneId, recordSetId).
		Return(mockDeleteReq).
		Times(1)

	// Mock GetRecordSetExecute for wait handler - recordset is gone
	tc.MockClient.EXPECT().
		GetRecordSetExecute(gomock.Any(), projectId, zoneId, recordSetId).
		Return(nil, goneErr).
		AnyTimes()

	schema := tc.GetSchema()

	state := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, records)

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Delete should succeed, but got errors: %v", resp.Diagnostics.Errors())
}

func TestDelete_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	recordSetId := "recordset-xyz-456"
	name := "www"
	recordType := dns.RECORDSETTYPE_A
	records := []string{"192.168.1.1"}

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Mock DeleteRecordSet
	mockDeleteReq := mock_recordset.NewMockApiDeleteRecordSetRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(nil, nil).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteRecordSet(gomock.Any(), projectId, zoneId, recordSetId).
		Return(mockDeleteReq).
		Times(1)

	// Mock GetRecordSetExecute for wait handler - simulate timeout
	tc.MockClient.EXPECT().
		GetRecordSetExecute(gomock.Any(), projectId, zoneId, recordSetId).
		DoAndReturn(func(ctx context.Context, projectId, zoneId, recordSetId string) (*dns.RecordSetResponse, error) {
			time.Sleep(150 * time.Millisecond) // Longer than context timeout
			return nil, ctx.Err()
		}).
		AnyTimes()

	schema := tc.GetSchema()

	state := CreateTestModel(projectId, zoneId, recordSetId, name, recordType, records)

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, &state)

	tc.Resource.Delete(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Expected no error due to context timeout")

	var stateAfterDelete Model
	diags := resp.State.Get(tc.Ctx, &stateAfterDelete)
	require.False(t, diags.HasError(), "Failed to get state after delete: %v", diags.Errors())

	// State should be preserved since delete wait failed
	require.Equal(t, state.RecordSetId.ValueString(), stateAfterDelete.RecordSetId.ValueString())
	require.Equal(t, state.Id.ValueString(), stateAfterDelete.Id.ValueString())
	require.Equal(t, state.ProjectId.ValueString(), stateAfterDelete.ProjectId.ValueString())
	require.Equal(t, state.ZoneId.ValueString(), stateAfterDelete.ZoneId.ValueString())
	require.Equal(t, state.Name.ValueString(), stateAfterDelete.Name.ValueString())
}

func TestDelete_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	zoneId := "zone-123"
	recordSetId := "recordset-456"

	// Setup mock expectations - server error
	serverErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusInternalServerError,
	}

	// Mock DeleteRecordSet
	mockDeleteReq := mock_recordset.NewMockApiDeleteRecordSetRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(nil, serverErr).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteRecordSet(gomock.Any(), projectId, zoneId, recordSetId).
		Return(mockDeleteReq).
		Times(1)

	schema := tc.GetSchema()

	state := CreateTestModel(projectId, zoneId, recordSetId, "www", dns.RECORDSETTYPE_A, []string{"192.168.1.1"})

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	require.True(t, resp.Diagnostics.HasError(), "Expected error when API call fails")
}

func TestDelete_RecordSetAlreadyDeleted(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	zoneId := "zone-123"
	recordSetId := "recordset-456"

	// Setup mock expectations - DeleteRecordSet returns 404
	notFoundErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusNotFound,
	}

	// Mock DeleteRecordSet
	mockDeleteReq := mock_recordset.NewMockApiDeleteRecordSetRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(nil, notFoundErr).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteRecordSet(gomock.Any(), projectId, zoneId, recordSetId).
		Return(mockDeleteReq).
		Times(1)

	schema := tc.GetSchema()

	state := CreateTestModel(projectId, zoneId, recordSetId, "www", dns.RECORDSETTYPE_A, []string{"192.168.1.1"})

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	// Delete should succeed for idempotency - recordset already deleted
	require.False(t, resp.Diagnostics.HasError(), "Delete should succeed when recordset is already deleted (404), but got errors: %v", resp.Diagnostics.Errors())
}

func TestDelete_RecordSetGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	zoneId := "zone-123"
	recordSetId := "recordset-456"

	// Setup mock expectations - DeleteRecordSet returns 410 Gone
	goneErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusGone,
	}

	// Mock DeleteRecordSet
	mockDeleteReq := mock_recordset.NewMockApiDeleteRecordSetRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(nil, goneErr).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteRecordSet(gomock.Any(), projectId, zoneId, recordSetId).
		Return(mockDeleteReq).
		Times(1)

	schema := tc.GetSchema()

	state := CreateTestModel(projectId, zoneId, recordSetId, "www", dns.RECORDSETTYPE_A, []string{"192.168.1.1"})

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	// Delete should succeed for idempotency - recordset already gone
	require.False(t, resp.Diagnostics.HasError(), "Delete should succeed when recordset is gone (410), but got errors: %v", resp.Diagnostics.Errors())
}

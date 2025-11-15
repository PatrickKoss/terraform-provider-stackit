package dns

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stackitcloud/stackit-sdk-go/core/oapierror"
	"github.com/stackitcloud/stackit-sdk-go/services/dns"
	mock_zone "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/dns/zone/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDelete_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	name := "my-test-zone"
	dnsName := "example.com."

	// Setup mock expectations - delete succeeds
	goneErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusGone,
	}

	// Mock DeleteZone
	mockDeleteReq := mock_zone.NewMockApiDeleteZoneRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(nil, nil).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteZone(gomock.Any(), projectId, zoneId).
		Return(mockDeleteReq).
		Times(1)

	// Mock GetZoneExecute for wait handler - zone is gone (returns ZoneResponse with error)
	tc.MockClient.EXPECT().
		GetZoneExecute(gomock.Any(), projectId, zoneId).
		Return(nil, goneErr).
		AnyTimes()

	schema := tc.GetSchema()

	state := Model{
		Id:        types.StringValue(fmt.Sprintf("%s,%s", projectId, zoneId)),
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		Name:      types.StringValue(name),
		DnsName:   types.StringValue(dnsName),
		Primaries: types.ListNull(types.StringType),
	}

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
	name := "my-test-zone"
	dnsName := "example.com."

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Mock DeleteZone
	mockDeleteReq := mock_zone.NewMockApiDeleteZoneRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(nil, nil).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteZone(gomock.Any(), projectId, zoneId).
		Return(mockDeleteReq).
		Times(1)

	// Mock GetZoneExecute for wait handler - simulate timeout
	tc.MockClient.EXPECT().
		GetZoneExecute(gomock.Any(), projectId, zoneId).
		DoAndReturn(func(ctx context.Context, projectId, zoneId string) (*dns.Zone, error) {
			time.Sleep(150 * time.Millisecond) // Longer than context timeout
			return nil, ctx.Err()
		}).
		AnyTimes()

	schema := tc.GetSchema()

	state := Model{
		Id:        types.StringValue(fmt.Sprintf("%s,%s", projectId, zoneId)),
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		Name:      types.StringValue(name),
		DnsName:   types.StringValue(dnsName),
		Primaries: types.ListNull(types.StringType),
	}

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, &state)

	tc.Resource.Delete(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Expected no error due to context timeout")

	var stateAfterDelete Model
	diags := resp.State.Get(tc.Ctx, &stateAfterDelete)
	require.False(t, diags.HasError(), "Failed to get state after delete: %v", diags.Errors())

	// State should be preserved since delete wait failed
	require.Equal(t, state.ZoneId.ValueString(), stateAfterDelete.ZoneId.ValueString())
	require.Equal(t, state.Id.ValueString(), stateAfterDelete.Id.ValueString())
	require.Equal(t, state.ProjectId.ValueString(), stateAfterDelete.ProjectId.ValueString())
	require.Equal(t, state.Name.ValueString(), stateAfterDelete.Name.ValueString())
	require.Equal(t, state.DnsName.ValueString(), stateAfterDelete.DnsName.ValueString())
}

func TestDelete_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	zoneId := "test-zone"

	// Setup mock expectations - server error
	serverErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusInternalServerError,
	}

	// Mock DeleteZone
	mockDeleteReq := mock_zone.NewMockApiDeleteZoneRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(nil, serverErr).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteZone(gomock.Any(), projectId, zoneId).
		Return(mockDeleteReq).
		Times(1)

	schema := tc.GetSchema()

	state := Model{
		Id:        types.StringValue("test-project,test-zone"),
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		Name:      types.StringValue("test-name"),
		DnsName:   types.StringValue("example.com."),
		Primaries: types.ListNull(types.StringType),
	}

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	require.True(t, resp.Diagnostics.HasError(), "Expected error when API call fails")
}

func TestDelete_ZoneAlreadyDeleted(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	zoneId := "test-zone"

	// Setup mock expectations - DeleteZone returns 404
	notFoundErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusNotFound,
	}

	// Mock DeleteZone
	mockDeleteReq := mock_zone.NewMockApiDeleteZoneRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(nil, notFoundErr).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteZone(gomock.Any(), projectId, zoneId).
		Return(mockDeleteReq).
		Times(1)

	schema := tc.GetSchema()

	state := Model{
		Id:        types.StringValue("test-project,test-zone"),
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		Name:      types.StringValue("test-name"),
		DnsName:   types.StringValue("example.com."),
		Primaries: types.ListNull(types.StringType),
	}

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	// Delete should succeed for idempotency - zone already deleted
	require.False(t, resp.Diagnostics.HasError(), "Delete should succeed when zone is already deleted (404), but got errors: %v", resp.Diagnostics.Errors())
}

func TestDelete_ZoneGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	zoneId := "test-zone"

	// Setup mock expectations - DeleteZone returns 410 Gone
	goneErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusGone,
	}

	// Mock DeleteZone
	mockDeleteReq := mock_zone.NewMockApiDeleteZoneRequest(tc.MockCtrl)
	mockDeleteReq.EXPECT().
		Execute().
		Return(nil, goneErr).
		Times(1)

	tc.MockClient.EXPECT().
		DeleteZone(gomock.Any(), projectId, zoneId).
		Return(mockDeleteReq).
		Times(1)

	schema := tc.GetSchema()

	state := Model{
		Id:        types.StringValue("test-project,test-zone"),
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		Name:      types.StringValue("test-name"),
		DnsName:   types.StringValue("example.com."),
		Primaries: types.ListNull(types.StringType),
	}

	req := DeleteRequest(tc.Ctx, schema, state)
	resp := DeleteResponse(tc.Ctx, schema, nil)

	tc.Resource.Delete(tc.Ctx, req, resp)

	// Delete should succeed for idempotency - zone already gone
	require.False(t, resp.Diagnostics.HasError(), "Delete should succeed when zone is gone (410), but got errors: %v", resp.Diagnostics.Errors())
}

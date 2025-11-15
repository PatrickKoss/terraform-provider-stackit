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
	mock_zone "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/dns/zone/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRead_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	name := "my-test-zone"
	dnsName := "example.com."

	// Setup mock expectations
	zoneResp := BuildZoneResponse(zoneId, name, dnsName)

	// Mock GetZone
	mockGetReq := mock_zone.NewMockApiGetZoneRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(zoneResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		GetZone(gomock.Any(), projectId, zoneId).
		Return(mockGetReq).
		Times(1)

	schema := tc.GetSchema()

	state := Model{
		Id:        types.StringValue(fmt.Sprintf("%s,%s", projectId, zoneId)),
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		// Other fields may be outdated or null
		Name:      types.StringNull(),
		DnsName:   types.StringNull(),
		Primaries: types.ListNull(types.StringType),
	}

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var refreshedState Model
	diags := resp.State.Get(tc.Ctx, &refreshedState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	require.Equal(t, zoneId, refreshedState.ZoneId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s", projectId, zoneId), refreshedState.Id.ValueString())
	require.Equal(t, projectId, refreshedState.ProjectId.ValueString())
	require.Equal(t, name, refreshedState.Name.ValueString())
	require.Equal(t, dnsName, refreshedState.DnsName.ValueString())

	require.False(t, refreshedState.Active.IsNull(), "Active should be set from API response")
	require.False(t, refreshedState.State.IsNull(), "State should be set from API response")
}

func TestRead_ZoneNotFound(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	zoneId := "non-existent-zone"

	// Setup GetZone to return 404
	notFoundErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusNotFound,
	}

	// Mock GetZone
	mockGetReq := mock_zone.NewMockApiGetZoneRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(nil, notFoundErr).
		Times(1)

	tc.MockClient.EXPECT().
		GetZone(gomock.Any(), projectId, zoneId).
		Return(mockGetReq).
		Times(1)

	schema := tc.GetSchema()

	state := Model{
		Id:        types.StringValue(fmt.Sprintf("%s,%s", projectId, zoneId)),
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		Primaries: types.ListNull(types.StringType),
	}

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should not error when zone not found, but got errors: %v", resp.Diagnostics.Errors())
}

func TestRead_ZoneGone(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	zoneId := "gone-zone"

	// Setup GetZone to return 410 Gone
	goneErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusGone,
	}

	// Mock GetZone
	mockGetReq := mock_zone.NewMockApiGetZoneRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(nil, goneErr).
		Times(1)

	tc.MockClient.EXPECT().
		GetZone(gomock.Any(), projectId, zoneId).
		Return(mockGetReq).
		Times(1)

	schema := tc.GetSchema()

	state := Model{
		Id:        types.StringValue(fmt.Sprintf("%s,%s", projectId, zoneId)),
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		Primaries: types.ListNull(types.StringType),
	}

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should not error when zone is gone, but got errors: %v", resp.Diagnostics.Errors())
}

func TestRead_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	zoneId := "test-zone"

	// Setup GetZone to return 500 error
	serverErr := &oapierror.GenericOpenAPIError{
		StatusCode: http.StatusInternalServerError,
	}

	// Mock GetZone
	mockGetReq := mock_zone.NewMockApiGetZoneRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(nil, serverErr).
		Times(1)

	tc.MockClient.EXPECT().
		GetZone(gomock.Any(), projectId, zoneId).
		Return(mockGetReq).
		Times(1)

	schema := tc.GetSchema()

	state := Model{
		Id:        types.StringValue("test-project,test-zone"),
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
	}

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
	name := "my-test-zone"
	dnsName := "example.com."

	// Setup mock expectations - zone ACL and DefaultTTL changed in cloud
	zone := BuildZone(zoneId, name, dnsName)
	zone.Acl = utils.Ptr("192.168.0.0/16")      // ACL changed
	zone.DefaultTTL = utils.Ptr(int64(7200))    // TTL changed
	zoneResp := &dns.ZoneResponse{Zone: zone}

	// Mock GetZone - returns drifted state
	mockGetReq := mock_zone.NewMockApiGetZoneRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(zoneResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		GetZone(gomock.Any(), projectId, zoneId).
		Return(mockGetReq).
		Times(1)

	schema := tc.GetSchema()

	state := Model{
		Id:         types.StringValue(fmt.Sprintf("%s,%s", projectId, zoneId)),
		ProjectId:  types.StringValue(projectId),
		ZoneId:     types.StringValue(zoneId),
		Name:       types.StringValue(name),
		DnsName:    types.StringValue(dnsName),
		Acl:        types.StringValue("0.0.0.0/0"),       // Old value
		DefaultTTL: types.Int64Value(3600),               // Old value
		Primaries:  types.ListNull(types.StringType),
	}

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var refreshedState Model
	diags := resp.State.Get(tc.Ctx, &refreshedState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify drift was detected and state was updated
	require.Equal(t, "192.168.0.0/16", refreshedState.Acl.ValueString())
	require.Equal(t, int64(7200), refreshedState.DefaultTTL.ValueInt64())
}

func TestRead_PrimariesOrderPreserved(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	name := "my-test-zone"
	dnsName := "example.com."
	primaries := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"}

	// Setup mock expectations with primaries
	zone := BuildZoneWithPrimaries(zoneId, name, dnsName, primaries)
	zoneResp := &dns.ZoneResponse{Zone: zone}

	// Mock GetZone
	mockGetReq := mock_zone.NewMockApiGetZoneRequest(tc.MockCtrl)
	mockGetReq.EXPECT().
		Execute().
		Return(zoneResp, nil).
		Times(1)

	tc.MockClient.EXPECT().
		GetZone(gomock.Any(), projectId, zoneId).
		Return(mockGetReq).
		Times(1)

	schema := tc.GetSchema()

	// State with primaries in different order
	differentOrderPrimaries := []string{"192.168.1.3", "192.168.1.1", "192.168.1.2"}
	primaryValues := make([]attr.Value, len(differentOrderPrimaries))
	for i, p := range differentOrderPrimaries {
		primaryValues[i] = types.StringValue(p)
	}
	primaryList, _ := types.ListValue(types.StringType, primaryValues)

	state := Model{
		Id:        types.StringValue(fmt.Sprintf("%s,%s", projectId, zoneId)),
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		Name:      types.StringValue(name),
		DnsName:   types.StringValue(dnsName),
		Primaries: primaryList,
	}

	req := ReadRequest(tc.Ctx, schema, state)
	resp := ReadResponse(schema)

	tc.Resource.Read(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Read should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var refreshedState Model
	diags := resp.State.Get(tc.Ctx, &refreshedState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify primaries order matches API response (not state order)
	require.False(t, refreshedState.Primaries.IsNull(), "Primaries should be set")

	var statePrimaries []string
	diags = refreshedState.Primaries.ElementsAs(tc.Ctx, &statePrimaries, false)
	require.False(t, diags.HasError(), "Failed to get primaries: %v", diags.Errors())
	require.Equal(t, primaries, statePrimaries, "Primaries order should match API response")
}

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

func TestUpdate_Success(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	name := "my-test-zone"
	dnsName := "example.com."

	// Setup mock expectations - update ACL
	zone := BuildZone(zoneId, name, dnsName)
	zone.Acl = utils.Ptr("192.168.0.0/16") // Updated ACL

	// Mock PartialUpdateZone
	mockUpdateReq := mock_zone.NewMockApiPartialUpdateZoneRequest(tc.MockCtrl)
	mockUpdateReq.EXPECT().
		PartialUpdateZonePayload(gomock.Any()).
		Return(mockUpdateReq).
		Times(1)
	mockUpdateReq.EXPECT().
		Execute().
		Return(nil).
		Times(1)

	tc.MockClient.EXPECT().
		PartialUpdateZone(gomock.Any(), projectId, zoneId).
		Return(mockUpdateReq).
		Times(1)

	// Mock GetZoneExecute for wait handler
	tc.MockClient.EXPECT().
		GetZoneExecute(gomock.Any(), projectId, zoneId).
		Return(&dns.ZoneResponse{Zone: zone}, nil).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()

	currentState := Model{
		Id:        types.StringValue(fmt.Sprintf("%s,%s", projectId, zoneId)),
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		Name:      types.StringValue(name),
		DnsName:   types.StringValue(dnsName),
		Acl:       types.StringValue("0.0.0.0/0"), // Old value
	}

	plannedState := Model{
		Id:        types.StringValue(fmt.Sprintf("%s,%s", projectId, zoneId)),
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		Name:      types.StringValue(name),
		DnsName:   types.StringValue(dnsName),
		Acl:       types.StringValue("192.168.0.0/16"), // New value
	}

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

	// Verify all fields match the updated values from GetZone
	require.Equal(t, zoneId, finalState.ZoneId.ValueString())
	require.Equal(t, fmt.Sprintf("%s,%s", projectId, zoneId), finalState.Id.ValueString())
	require.Equal(t, projectId, finalState.ProjectId.ValueString())
	require.Equal(t, name, finalState.Name.ValueString())
	require.Equal(t, "192.168.0.0/16", finalState.Acl.ValueString())
}

func TestUpdate_ContextCanceledDuringWait(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	name := "my-test-zone"
	dnsName := "example.com."

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tc.Ctx = ctx

	// Mock PartialUpdateZone
	mockUpdateReq := mock_zone.NewMockApiPartialUpdateZoneRequest(tc.MockCtrl)
	mockUpdateReq.EXPECT().
		PartialUpdateZonePayload(gomock.Any()).
		Return(mockUpdateReq).
		Times(1)
	mockUpdateReq.EXPECT().
		Execute().
		Return(nil).
		Times(1)

	tc.MockClient.EXPECT().
		PartialUpdateZone(gomock.Any(), projectId, zoneId).
		Return(mockUpdateReq).
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

	currentState := Model{
		Id:        types.StringValue(fmt.Sprintf("%s,%s", projectId, zoneId)),
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		Name:      types.StringValue(name),
		DnsName:   types.StringValue(dnsName),
		Acl:       types.StringValue("0.0.0.0/0"),
	}

	plannedState := Model{
		Id:        types.StringValue(fmt.Sprintf("%s,%s", projectId, zoneId)),
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		Name:      types.StringValue(name),
		DnsName:   types.StringValue(dnsName),
		Acl:       types.StringValue("192.168.0.0/16"),
	}

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
	if !stateAfterUpdate.Acl.IsNull() {
		actualAcl := stateAfterUpdate.Acl.ValueString()
		require.NotEqual(t, "192.168.0.0/16", actualAcl, "BUG: State has NEW ACL even though update wait failed!")
		require.Equal(t, "0.0.0.0/0", actualAcl)
	}

	require.Equal(t, currentState.Id.ValueString(), stateAfterUpdate.Id.ValueString())
	require.Equal(t, currentState.ProjectId.ValueString(), stateAfterUpdate.ProjectId.ValueString())
	require.Equal(t, currentState.ZoneId.ValueString(), stateAfterUpdate.ZoneId.ValueString())
	require.Equal(t, currentState.Name.ValueString(), stateAfterUpdate.Name.ValueString())
	require.Equal(t, currentState.DnsName.ValueString(), stateAfterUpdate.DnsName.ValueString())
}

func TestUpdate_APICallFails(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project"
	zoneId := "test-zone"

	// Setup mock expectations - PartialUpdateZone fails
	apiErr := fmt.Errorf("API error")

	mockUpdateReq := mock_zone.NewMockApiPartialUpdateZoneRequest(tc.MockCtrl)
	mockUpdateReq.EXPECT().
		PartialUpdateZonePayload(gomock.Any()).
		Return(mockUpdateReq).
		Times(1)
	mockUpdateReq.EXPECT().
		Execute().
		Return(apiErr).
		Times(1)

	tc.MockClient.EXPECT().
		PartialUpdateZone(gomock.Any(), projectId, zoneId).
		Return(mockUpdateReq).
		Times(1)

	// Prepare request
	schema := tc.GetSchema()

	currentState := Model{
		Id:        types.StringValue("test-project,test-zone"),
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		Name:      types.StringValue("test-name"),
		DnsName:   types.StringValue("example.com."),
		Acl:       types.StringValue("0.0.0.0/0"),
	}

	plannedState := Model{
		Id:        types.StringValue("test-project,test-zone"),
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		Name:      types.StringValue("test-name"),
		DnsName:   types.StringValue("example.com."),
		Acl:       types.StringValue("192.168.0.0/16"),
	}

	req := UpdateRequest(tc.Ctx, schema, currentState, plannedState)
	resp := UpdateResponse(tc.Ctx, schema, nil)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	require.True(t, resp.Diagnostics.HasError(), "Expected error when API call fails")
}

func TestUpdate_ChangePrimaries(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	name := "my-test-zone"
	dnsName := "example.com."
	oldPrimaries := []string{"192.168.1.1", "192.168.1.2"}
	newPrimaries := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"}

	// Setup mock expectations
	zone := BuildZoneWithPrimaries(zoneId, name, dnsName, newPrimaries)

	// Mock PartialUpdateZone
	mockUpdateReq := mock_zone.NewMockApiPartialUpdateZoneRequest(tc.MockCtrl)
	mockUpdateReq.EXPECT().
		PartialUpdateZonePayload(gomock.Any()).
		Return(mockUpdateReq).
		Times(1)
	mockUpdateReq.EXPECT().
		Execute().
		Return(nil).
		Times(1)

	tc.MockClient.EXPECT().
		PartialUpdateZone(gomock.Any(), projectId, zoneId).
		Return(mockUpdateReq).
		Times(1)

	// Mock GetZoneExecute for wait handler
	tc.MockClient.EXPECT().
		GetZoneExecute(gomock.Any(), projectId, zoneId).
		Return(&dns.ZoneResponse{Zone: zone}, nil).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()

	// Current state with old primaries
	oldPrimaryValues := make([]attr.Value, len(oldPrimaries))
	for i, p := range oldPrimaries {
		oldPrimaryValues[i] = types.StringValue(p)
	}
	oldPrimaryList, _ := types.ListValue(types.StringType, oldPrimaryValues)

	currentState := Model{
		Id:        types.StringValue(fmt.Sprintf("%s,%s", projectId, zoneId)),
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		Name:      types.StringValue(name),
		DnsName:   types.StringValue(dnsName),
		Primaries: oldPrimaryList,
		Type:      types.StringValue(string(dns.ZONETYPE_SECONDARY)),
	}

	// Planned state with new primaries
	newPrimaryValues := make([]attr.Value, len(newPrimaries))
	for i, p := range newPrimaries {
		newPrimaryValues[i] = types.StringValue(p)
	}
	newPrimaryList, _ := types.ListValue(types.StringType, newPrimaryValues)

	plannedState := Model{
		Id:        types.StringValue(fmt.Sprintf("%s,%s", projectId, zoneId)),
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		Name:      types.StringValue(name),
		DnsName:   types.StringValue(dnsName),
		Primaries: newPrimaryList,
		Type:      types.StringValue(string(dns.ZONETYPE_SECONDARY)),
	}

	req := UpdateRequest(tc.Ctx, schema, currentState, plannedState)
	resp := UpdateResponse(tc.Ctx, schema, nil)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	require.False(t, resp.Diagnostics.HasError(), "Update should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify primaries were updated
	require.False(t, finalState.Primaries.IsNull(), "Primaries should be set")

	var statePrimaries []string
	diags = finalState.Primaries.ElementsAs(tc.Ctx, &statePrimaries, false)
	require.False(t, diags.HasError(), "Failed to get primaries: %v", diags.Errors())
	require.Equal(t, newPrimaries, statePrimaries, "Primaries should be updated")
}

func TestUpdate_ChangeDescription(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	// Test data
	projectId := "test-project-123"
	zoneId := "zone-abc-123"
	name := "my-test-zone"
	dnsName := "example.com."

	// Setup mock expectations - change description
	zone := BuildZone(zoneId, name, dnsName)
	zone.Description = utils.Ptr("Updated zone description")

	// Mock PartialUpdateZone
	mockUpdateReq := mock_zone.NewMockApiPartialUpdateZoneRequest(tc.MockCtrl)
	mockUpdateReq.EXPECT().
		PartialUpdateZonePayload(gomock.Any()).
		Return(mockUpdateReq).
		Times(1)
	mockUpdateReq.EXPECT().
		Execute().
		Return(nil).
		Times(1)

	tc.MockClient.EXPECT().
		PartialUpdateZone(gomock.Any(), projectId, zoneId).
		Return(mockUpdateReq).
		Times(1)

	// Mock GetZoneExecute for wait handler
	tc.MockClient.EXPECT().
		GetZoneExecute(gomock.Any(), projectId, zoneId).
		Return(&dns.ZoneResponse{Zone: zone}, nil).
		AnyTimes()

	// Prepare request
	schema := tc.GetSchema()

	currentState := Model{
		Id:          types.StringValue(fmt.Sprintf("%s,%s", projectId, zoneId)),
		ProjectId:   types.StringValue(projectId),
		ZoneId:      types.StringValue(zoneId),
		Name:        types.StringValue(name),
		DnsName:     types.StringValue(dnsName),
		Description: types.StringValue("Original description"),
	}

	plannedState := Model{
		Id:          types.StringValue(fmt.Sprintf("%s,%s", projectId, zoneId)),
		ProjectId:   types.StringValue(projectId),
		ZoneId:      types.StringValue(zoneId),
		Name:        types.StringValue(name),
		DnsName:     types.StringValue(dnsName),
		Description: types.StringValue("Updated zone description"),
	}

	req := UpdateRequest(tc.Ctx, schema, currentState, plannedState)
	resp := UpdateResponse(tc.Ctx, schema, nil)

	// Execute Update
	tc.Resource.Update(tc.Ctx, req, resp)

	// Assertions
	require.False(t, resp.Diagnostics.HasError(), "Update should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var finalState Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify description was changed
	require.Equal(t, "Updated zone description", finalState.Description.ValueString())
}

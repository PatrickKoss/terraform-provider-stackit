package network

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stackitcloud/stackit-sdk-go/core/utils"
	"github.com/stackitcloud/stackit-sdk-go/services/iaas"
	mock_network_v1 "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/iaas/network/mock/v1"
	networkModel "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/iaas/network/utils/model"
	internalUtils "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/utils"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestCreate_V1_ListFieldsWithUnknownElements tests that lists with unknown elements are properly handled
func TestCreate_V1_ListFieldsWithUnknownElements(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	networkId := "network-abc-123"
	networkName := "my-test-network"
	ipv4Prefix := "10.0.0.0/24"
	gateway := "10.0.0.1"
	publicIP := "1.2.3.4"
	prefixes := []string{ipv4Prefix}
	nameservers := []string{"8.8.8.8"}

	// Create a model where some list fields are unknown
	model := CreateTestModel(projectId, "", networkName, ipv4Prefix)
	// Override some lists to be unknown
	model.IPv6Nameservers = types.ListUnknown(types.StringType)
	model.IPv6Prefixes = types.ListUnknown(types.StringType)

	networkResp := BuildNetwork(networkId, networkName, gateway, publicIP, prefixes, nameservers)

	mockCreateReq := mock_network_v1.NewMockApiCreateNetworkRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateNetworkPayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(&iaas.Network{NetworkId: utils.Ptr(networkId)}, nil).
		Times(1)

	tc.MockClient.EXPECT().
		CreateNetwork(gomock.Any(), projectId).
		Return(mockCreateReq).
		Times(1)

	tc.MockClient.EXPECT().
		GetNetworkExecute(gomock.Any(), projectId, networkId).
		Return(networkResp, nil).
		AnyTimes()

	schema := tc.GetSchema()
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Create should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var finalState networkModel.Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// Verify no unknown fields remain
	assertNoUnknownFields(t, finalState)
}

// assertNoUnknownFields verifies that no field in the model is Unknown
// This is the key assertion for detecting the reported bug
func assertNoUnknownFields(t *testing.T, model networkModel.Model) {
	v := reflect.ValueOf(model)
	modelType := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := modelType.Field(i)

		if !field.CanInterface() {
			continue
		}

		// Check if the field has an IsUnknown method
		isUnknownMethod := field.MethodByName("IsUnknown")
		if !isUnknownMethod.IsValid() {
			continue
		}

		// Call IsUnknown()
		result := isUnknownMethod.Call(nil)
		if len(result) == 0 {
			continue
		}

		isUnknown := result[0].Bool()
		require.False(t, isUnknown, "BUG DETECTED: Field %s is Unknown after Create. All fields must be known (either with a value or null) after apply operation. This is the bug reported: 'After the apply operation, the provider still indicated an unknown value'", fieldType.Name)
	}
}

// TestSetModelFieldsToNull_DirectCall tests the SetModelFieldsToNull utility function directly
// This helps isolate whether the bug is in the utility function itself
func TestSetModelFieldsToNull_DirectCall(t *testing.T) {
	ctx := context.Background()

	// Create a model with various unknown fields
	model := networkModel.Model{
		ProjectId: types.StringValue("test-project"),
		Name:      types.StringValue("test-network"),
		NetworkId: types.StringValue("network-123"),
		Id:        types.StringValue("test-project,network-123"),
		// Unknown fields that should be converted to null
		IPv4Gateway:      types.StringUnknown(),
		IPv6Gateway:      types.StringUnknown(),
		IPv6Prefix:       types.StringUnknown(),
		IPv6PrefixLength: types.Int64Unknown(),
		PublicIP:         types.StringUnknown(),
		Routed:           types.BoolUnknown(),
		Region:           types.StringUnknown(),
		RoutingTableID:   types.StringUnknown(),
		// Unknown lists
		Nameservers:     types.ListUnknown(types.StringType),
		IPv4Nameservers: types.ListUnknown(types.StringType),
		IPv6Nameservers: types.ListUnknown(types.StringType),
		Prefixes:        types.ListUnknown(types.StringType),
		IPv4Prefixes:    types.ListUnknown(types.StringType),
		IPv6Prefixes:    types.ListUnknown(types.StringType),
		// Unknown map
		Labels: types.MapUnknown(types.StringType),
		// Fields that should remain unchanged
		IPv4Prefix:       types.StringValue("10.0.0.0/24"),
		IPv4PrefixLength: types.Int64Value(24),
		NoIPv4Gateway:    types.BoolValue(false),
		NoIPv6Gateway:    types.BoolValue(false),
	}

	// Call SetModelFieldsToNull
	err := internalUtils.SetModelFieldsToNull(ctx, &model)
	require.NoError(t, err, "SetModelFieldsToNull should not error")

	// Verify all unknown fields are now null
	require.True(t, model.IPv4Gateway.IsNull(), "IPv4Gateway should be null, not unknown")
	require.False(t, model.IPv4Gateway.IsUnknown(), "IPv4Gateway should not be unknown")

	require.True(t, model.IPv6Gateway.IsNull(), "IPv6Gateway should be null, not unknown")
	require.False(t, model.IPv6Gateway.IsUnknown(), "IPv6Gateway should not be unknown")

	require.True(t, model.IPv6Prefix.IsNull(), "IPv6Prefix should be null, not unknown")
	require.False(t, model.IPv6Prefix.IsUnknown(), "IPv6Prefix should not be unknown after SetModelFieldsToNull")

	require.True(t, model.IPv6PrefixLength.IsNull(), "IPv6PrefixLength should be null, not unknown")
	require.False(t, model.IPv6PrefixLength.IsUnknown(), "IPv6PrefixLength should not be unknown after SetModelFieldsToNull")

	require.True(t, model.PublicIP.IsNull())
	require.False(t, model.PublicIP.IsUnknown())

	require.True(t, model.Routed.IsNull())
	require.False(t, model.Routed.IsUnknown())

	require.True(t, model.Nameservers.IsNull())
	require.False(t, model.Nameservers.IsUnknown(), "Nameservers list should not be unknown after SetModelFieldsToNull")

	require.True(t, model.IPv6Nameservers.IsNull())
	require.False(t, model.IPv6Nameservers.IsUnknown(), "IPv6Nameservers list should not be unknown after SetModelFieldsToNull")

	require.True(t, model.Labels.IsNull())
	require.False(t, model.Labels.IsUnknown(), "Labels map should not be unknown after SetModelFieldsToNull")

	// Verify fields that should remain unchanged are not affected
	require.Equal(t, "10.0.0.0/24", model.IPv4Prefix.ValueString())
	require.False(t, model.IPv4Prefix.IsUnknown())
	require.False(t, model.IPv4Prefix.IsNull())

	require.Equal(t, int64(24), model.IPv4PrefixLength.ValueInt64())
	require.False(t, model.IPv4PrefixLength.IsUnknown())
	require.False(t, model.IPv4PrefixLength.IsNull())

	// Finally, assert no unknown fields remain in the entire model
	assertNoUnknownFields(t, model)
}

// TestSetModelFieldsToNull_ComplexScenario tests edge cases with the utility function
func TestSetModelFieldsToNull_ComplexScenario(t *testing.T) {
	ctx := context.Background()

	// Scenario: Mix of known values, nulls, and unknowns
	model := networkModel.Model{
		ProjectId: types.StringValue("proj-1"),
		NetworkId: types.StringValue("net-1"),
		Id:        types.StringValue("proj-1,net-1"),
		Name:      types.StringValue("my-network"),
		// Some fields are already null
		IPv4Gateway: types.StringNull(),
		IPv6Gateway: types.StringNull(),
		// Some are unknown
		IPv6Prefix:       types.StringUnknown(),
		IPv6PrefixLength: types.Int64Unknown(),
		PublicIP:         types.StringUnknown(),
		// Some have values
		IPv4Prefix:       types.StringValue("10.0.0.0/24"),
		IPv4PrefixLength: types.Int64Value(24),
		NoIPv4Gateway:    types.BoolValue(true),
		NoIPv6Gateway:    types.BoolValue(false),
		// Mixed list states
		Nameservers:     types.ListNull(types.StringType),
		IPv4Nameservers: types.ListUnknown(types.StringType),
		IPv6Nameservers: types.ListUnknown(types.StringType),
		Prefixes:        types.ListNull(types.StringType),
		IPv4Prefixes:    types.ListNull(types.StringType),
		IPv6Prefixes:    types.ListUnknown(types.StringType),
		// Map states
		Labels:         types.MapUnknown(types.StringType),
		Routed:         types.BoolNull(),
		Region:         types.StringNull(),
		RoutingTableID: types.StringUnknown(),
	}

	err := internalUtils.SetModelFieldsToNull(ctx, &model)
	require.NoError(t, err)

	// All unknown fields should now be null
	assertNoUnknownFields(t, model)

	// Fields that were already null should remain null
	require.True(t, model.IPv4Gateway.IsNull())
	require.True(t, model.IPv6Gateway.IsNull())
	require.True(t, model.Nameservers.IsNull())

	// Fields that had values should keep their values
	require.Equal(t, "10.0.0.0/24", model.IPv4Prefix.ValueString())
	require.Equal(t, int64(24), model.IPv4PrefixLength.ValueInt64())
	require.True(t, model.NoIPv4Gateway.ValueBool())
	require.False(t, model.NoIPv6Gateway.ValueBool())
}

func TestCreate_V1_ReproduceBug_IPv6Fields(t *testing.T) {
	tc := NewTestContext(t)
	defer tc.Close()

	projectId := "test-project-123"
	networkId := "network-abc-123"
	networkName := "my-test-network"
	ipv4Prefix := "10.0.0.0/24"
	gateway := "10.0.0.1"
	publicIP := "1.2.3.4"
	prefixes := []string{ipv4Prefix}
	nameservers := []string{"8.8.8.8"}

	// Create model simulating user configuration where IPv6 fields are not provided
	// In Terraform, optional computed fields start as Unknown
	model := CreateTestModel(projectId, "", networkName, ipv4Prefix)
	// Explicitly set IPv6 fields to unknown to simulate the initial plan state
	model.IPv6Prefix = types.StringUnknown()
	model.IPv6PrefixLength = types.Int64Unknown()
	model.IPv6Gateway = types.StringUnknown()
	model.IPv6Nameservers = types.ListUnknown(types.StringType)
	model.IPv6Prefixes = types.ListUnknown(types.StringType)

	// API response does NOT include IPv6 data (it's null/absent)
	networkResp := BuildNetwork(networkId, networkName, gateway, publicIP, prefixes, nameservers)

	mockCreateReq := mock_network_v1.NewMockApiCreateNetworkRequest(tc.MockCtrl)
	mockCreateReq.EXPECT().
		CreateNetworkPayload(gomock.Any()).
		Return(mockCreateReq).
		Times(1)
	mockCreateReq.EXPECT().
		Execute().
		Return(&iaas.Network{NetworkId: utils.Ptr(networkId)}, nil).
		Times(1)

	tc.MockClient.EXPECT().
		CreateNetwork(gomock.Any(), projectId).
		Return(mockCreateReq).
		Times(1)

	tc.MockClient.EXPECT().
		GetNetworkExecute(gomock.Any(), projectId, networkId).
		Return(networkResp, nil).
		AnyTimes()

	schema := tc.GetSchema()
	req := CreateRequest(tc.Ctx, schema, model)
	resp := CreateResponse(schema)

	tc.Resource.Create(tc.Ctx, req, resp)

	require.False(t, resp.Diagnostics.HasError(), "Create should succeed, but got errors: %v", resp.Diagnostics.Errors())

	var finalState networkModel.Model
	diags := resp.State.Get(tc.Ctx, &finalState)
	require.False(t, diags.HasError(), "Failed to get state: %v", diags.Errors())

	// This is where the bug should be caught
	// IPv6 fields should be null (not unknown) after create
	if finalState.IPv6Prefix.IsUnknown() {
		t.Errorf("BUG REPRODUCED: IPv6Prefix is still unknown after Create. Expected null or known value.")
	}
	if finalState.IPv6PrefixLength.IsUnknown() {
		t.Errorf("BUG REPRODUCED: IPv6PrefixLength is still unknown after Create. Expected null or known value.")
	}
	if finalState.IPv6Gateway.IsUnknown() {
		t.Errorf("BUG REPRODUCED: IPv6Gateway is still unknown after Create. Expected null or known value.")
	}
	if finalState.IPv6Nameservers.IsUnknown() {
		t.Errorf("BUG REPRODUCED: IPv6Nameservers is still unknown after Create. Expected null or known value.")
	}
	if finalState.IPv6Prefixes.IsUnknown() {
		t.Errorf("BUG REPRODUCED: IPv6Prefixes is still unknown after Create. Expected null or known value.")
	}

	// General check for all fields
	assertNoUnknownFields(t, finalState)

	// Document expected behavior
	t.Log(fmt.Sprintf("IPv6Prefix - IsUnknown: %v, IsNull: %v", finalState.IPv6Prefix.IsUnknown(), finalState.IPv6Prefix.IsNull()))
	t.Log(fmt.Sprintf("IPv6PrefixLength - IsUnknown: %v, IsNull: %v", finalState.IPv6PrefixLength.IsUnknown(), finalState.IPv6PrefixLength.IsNull()))
}

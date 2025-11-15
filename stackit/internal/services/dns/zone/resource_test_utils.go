package dns

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	mock_zone "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/dns/zone/mock"
	"github.com/stackitcloud/stackit-sdk-go/core/utils"
	"github.com/stackitcloud/stackit-sdk-go/services/dns"
	"go.uber.org/mock/gomock"
)

// TestContext holds common test setup
type TestContext struct {
	T          *testing.T
	MockCtrl   *gomock.Controller
	MockClient *mock_zone.MockDefaultApi
	Resource   *zoneResource
	Ctx        context.Context
	CancelFunc context.CancelFunc
}

// NewTestContext creates a new test context with mock client
func NewTestContext(t *testing.T) *TestContext {
	ctrl := gomock.NewController(t)
	mockClient := mock_zone.NewMockDefaultApi(ctrl)

	resource := &zoneResource{
		client: mockClient,
	}

	return &TestContext{
		T:          t,
		MockCtrl:   ctrl,
		MockClient: mockClient,
		Resource:   resource,
		Ctx:        context.Background(),
	}
}

// Close cleans up the test context
func (tc *TestContext) Close() {
	if tc.CancelFunc != nil {
		tc.CancelFunc()
	}
	tc.MockCtrl.Finish()
}

// GetSchema returns the resource schema
func (tc *TestContext) GetSchema() resource.SchemaResponse {
	schemaResp := resource.SchemaResponse{}
	tc.Resource.Schema(tc.Ctx, resource.SchemaRequest{}, &schemaResp)
	return schemaResp
}

// BuildZoneResponse creates a dns.ZoneResponse with full zone details
func BuildZoneResponse(zoneId, name, dnsName string) *dns.ZoneResponse {
	return &dns.ZoneResponse{
		Zone: BuildZone(zoneId, name, dnsName),
	}
}

// BuildZone creates a dns.Zone with the given fields
func BuildZone(zoneId, name, dnsName string) *dns.Zone {
	return &dns.Zone{
		Id:                utils.Ptr(zoneId),
		Name:              utils.Ptr(name),
		DnsName:           utils.Ptr(dnsName),
		Description:       utils.Ptr("Test zone description"),
		Acl:               utils.Ptr("0.0.0.0/0"),
		Active:            utils.Ptr(true),
		ContactEmail:      utils.Ptr("contact@example.com"),
		DefaultTTL:        utils.Ptr(int64(3600)),
		ExpireTime:        utils.Ptr(int64(604800)),
		IsReverseZone:     utils.Ptr(false),
		NegativeCache:     utils.Ptr(int64(300)),
		PrimaryNameServer: utils.Ptr("ns1.example.com"),
		Primaries:         &[]string{},
		RecordCount:       utils.Ptr(int64(5)),
		RefreshTime:       utils.Ptr(int64(86400)),
		RetryTime:         utils.Ptr(int64(7200)),
		SerialNumber:      utils.Ptr(int64(1)),
		Type:              dns.ZONETYPE_PRIMARY.Ptr(),
		Visibility:        dns.ZONEVISIBILITY_PUBLIC.Ptr(),
		State:             dns.ZONESTATE_CREATING.Ptr(),
	}
}

// BuildZoneWithPrimaries creates a dns.Zone with primaries list
func BuildZoneWithPrimaries(zoneId, name, dnsName string, primaries []string) *dns.Zone {
	zone := BuildZone(zoneId, name, dnsName)
	zone.Primaries = &primaries
	zone.Type = dns.ZONETYPE_SECONDARY.Ptr()
	return zone
}

// BuildCreateZonePayload creates a CreateZonePayload
func BuildCreateZonePayload(name, dnsName string) *dns.CreateZonePayload {
	return &dns.CreateZonePayload{
		Name:    utils.Ptr(name),
		DnsName: utils.Ptr(dnsName),
	}
}

// CreateTestModel creates a test model with common values
func CreateTestModel(projectId, zoneId, name, dnsName string) Model {
	return Model{
		ProjectId: types.StringValue(projectId),
		ZoneId:    types.StringValue(zoneId),
		Name:      types.StringValue(name),
		DnsName:   types.StringValue(dnsName),
	}
}

// CreateRequest creates a test Create request
func CreateRequest(ctx context.Context, schema resource.SchemaResponse, model Model) resource.CreateRequest {
	req := resource.CreateRequest{}
	req.Plan = tfsdk.Plan{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.Plan.Set(ctx, model)
	return req
}

// CreateResponse creates a test Create response
func CreateResponse(schema resource.SchemaResponse) *resource.CreateResponse {
	resp := &resource.CreateResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	return resp
}

// UpdateRequest creates a test Update request
func UpdateRequest(ctx context.Context, schema resource.SchemaResponse, currentState, plannedState Model) resource.UpdateRequest {
	req := resource.UpdateRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.Plan = tfsdk.Plan{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, currentState)
	req.Plan.Set(ctx, plannedState)
	return req
}

// UpdateResponse creates a test Update response
// Optionally initialize with current state to simulate Terraform framework behavior
func UpdateResponse(ctx context.Context, schema resource.SchemaResponse, currentState *Model) *resource.UpdateResponse {
	resp := &resource.UpdateResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	// Initialize with current state to simulate framework behavior
	// When Update errors without calling State.Set(), this state is preserved
	if currentState != nil {
		resp.State.Set(ctx, *currentState)
	}
	return resp
}

// DeleteRequest creates a test Delete request
func DeleteRequest(ctx context.Context, schema resource.SchemaResponse, state Model) resource.DeleteRequest {
	req := resource.DeleteRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, state)
	return req
}

// DeleteResponse creates a test Delete response
// Optionally initialize with current state to simulate Terraform framework behavior
func DeleteResponse(ctx context.Context, schema resource.SchemaResponse, currentState *Model) *resource.DeleteResponse {
	resp := &resource.DeleteResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	// Initialize with current state to simulate framework behavior
	// When Delete errors without calling State.RemoveResource(), this state is preserved
	if currentState != nil {
		resp.State.Set(ctx, *currentState)
	}
	return resp
}

// ReadRequest creates a test Read request
func ReadRequest(ctx context.Context, schema resource.SchemaResponse, state Model) resource.ReadRequest {
	req := resource.ReadRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, state)
	return req
}

// ReadResponse creates a test Read response
func ReadResponse(schema resource.SchemaResponse) *resource.ReadResponse {
	resp := &resource.ReadResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	return resp
}

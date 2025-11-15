package dns

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stackitcloud/stackit-sdk-go/core/utils"
	"github.com/stackitcloud/stackit-sdk-go/services/dns"
	mock_recordset "github.com/stackitcloud/terraform-provider-stackit/stackit/internal/services/dns/recordset/mock"
	"go.uber.org/mock/gomock"
)

// TestContext holds common test setup
type TestContext struct {
	T          *testing.T
	MockCtrl   *gomock.Controller
	MockClient *mock_recordset.MockDefaultApi
	Resource   *recordSetResource
	Ctx        context.Context
	CancelFunc context.CancelFunc
}

// NewTestContext creates a new test context with mock client
func NewTestContext(t *testing.T) *TestContext {
	ctrl := gomock.NewController(t)
	mockClient := mock_recordset.NewMockDefaultApi(ctrl)

	resource := &recordSetResource{
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

// BuildRecordSetResponse creates a dns.RecordSetResponse with full recordset details
func BuildRecordSetResponse(recordSetId, name, zoneId string, recordType dns.RecordSetTypes, records []string) *dns.RecordSetResponse {
	return &dns.RecordSetResponse{
		Rrset: BuildRecordSet(recordSetId, name, zoneId, recordType, records),
	}
}

// BuildRecordSet creates a dns.RecordSet with the given fields
func BuildRecordSet(recordSetId, name, zoneId string, recordType dns.RecordSetTypes, records []string) *dns.RecordSet {
	recordList := make([]dns.Record, len(records))
	for i, content := range records {
		recordList[i] = dns.Record{
			Content: utils.Ptr(content),
		}
	}

	return &dns.RecordSet{
		Id:      utils.Ptr(recordSetId),
		Name:    utils.Ptr(name),
		Type:    &recordType,
		Records: &recordList,
		Ttl:     utils.Ptr(int64(3600)),
		Active:  utils.Ptr(true),
		Comment: utils.Ptr("Test record set"),
		Error:   utils.Ptr(""),
		State:   dns.RECORDSETSTATE_CREATE_SUCCEEDED.Ptr(),
	}
}

// BuildRecordSetWithMultipleRecords creates a dns.RecordSet with multiple records
func BuildRecordSetWithMultipleRecords(recordSetId, name, zoneId string, recordType dns.RecordSetTypes, records []string) *dns.RecordSet {
	return BuildRecordSet(recordSetId, name, zoneId, recordType, records)
}

// BuildZone creates a dns.Zone for zone validation mocking
func BuildZone(zoneId, name, dnsName string) *dns.Zone {
	return &dns.Zone{
		Id:      utils.Ptr(zoneId),
		Name:    utils.Ptr(name),
		DnsName: utils.Ptr(dnsName),
		Active:  utils.Ptr(true),
		State:   dns.ZONESTATE_CREATE_SUCCEEDED.Ptr(),
	}
}

// BuildZoneResponse creates a dns.ZoneResponse for zone validation mocking
func BuildZoneResponse(zoneId, name, dnsName string) *dns.ZoneResponse {
	return &dns.ZoneResponse{
		Zone: BuildZone(zoneId, name, dnsName),
	}
}

// CreateTestModel creates a test model with common values
func CreateTestModel(projectId, zoneId, recordSetId, name string, recordType dns.RecordSetTypes, records []string) Model {
	recordValues := make([]attr.Value, len(records))
	for i, record := range records {
		recordValues[i] = types.StringValue(record)
	}
	recordsList, _ := types.ListValue(types.StringType, recordValues)

	return Model{
		ProjectId:   types.StringValue(projectId),
		ZoneId:      types.StringValue(zoneId),
		RecordSetId: types.StringValue(recordSetId),
		Name:        types.StringValue(name),
		Type:        types.StringValue(string(recordType)),
		Records:     recordsList,
		TTL:         types.Int64Value(3600),
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

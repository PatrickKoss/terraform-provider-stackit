package cdn

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stackitcloud/stackit-sdk-go/core/config"
	"github.com/stackitcloud/stackit-sdk-go/core/utils"
	"github.com/stackitcloud/stackit-sdk-go/services/cdn"
)

// TestContext holds common test setup
type TestContext struct {
	T          *testing.T
	Server     *httptest.Server
	Client     *cdn.APIClient
	Resource   *distributionResource
	Router     *mux.Router
	Ctx        context.Context
	CancelFunc context.CancelFunc
}

// NewTestContext creates a new test context with mock server
func NewTestContext(t *testing.T) *TestContext {
	router := mux.NewRouter()
	server := httptest.NewServer(router)

	client, err := cdn.NewAPIClient(
		config.WithEndpoint(server.URL),
		config.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	resource := &distributionResource{
		client: client,
	}

	return &TestContext{
		T:        t,
		Server:   server,
		Client:   client,
		Resource: resource,
		Router:   router,
		Ctx:      context.Background(),
	}
}

// Close cleans up the test context
func (tc *TestContext) Close() {
	if tc.CancelFunc != nil {
		tc.CancelFunc()
	}
	tc.Server.Close()
}

// GetSchema returns the resource schema
func (tc *TestContext) GetSchema() resource.SchemaResponse {
	schemaResp := resource.SchemaResponse{}
	tc.Resource.Schema(tc.Ctx, resource.SchemaRequest{}, &schemaResp)
	return schemaResp
}

// SetupCreateDistributionHandler adds mock handler for Create API
func (tc *TestContext) SetupCreateDistributionHandler(distributionId string, callCounter *int) {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)

		resp := cdn.CreateDistributionResponse{
			Distribution: &cdn.Distribution{
				Id: utils.Ptr(distributionId),
			},
		}
		respBytes, _ := json.Marshal(resp)
		w.Write(respBytes)
	}).Methods("POST")
}

// SetupGetDistributionHandler adds mock handler for Get API
func (tc *TestContext) SetupGetDistributionHandler(distributionId, projectId, status string) {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Create response using SDK types
		now := time.Now()
		statusEnum, _ := cdn.NewDistributionStatusFromValue(cdn.DistributionStatus(status))

		distribution := &cdn.Distribution{
			Id:        utils.Ptr(distributionId),
			ProjectId: utils.Ptr(projectId),
			Status:    statusEnum,
			CreatedAt: &now,
			UpdatedAt: &now,
			Config: &cdn.Config{
				Backend: &cdn.ConfigBackend{
					HttpBackend: &cdn.HttpBackend{
						Type:                 utils.Ptr("http"),
						OriginUrl:            utils.Ptr("https://example.com"),
						OriginRequestHeaders: &map[string]string{},
					},
				},
				Regions:          &[]cdn.Region{cdn.REGION_EU},
				BlockedCountries: &[]string{},
				BlockedIPs:       &[]string{},
				Waf:              cdn.NewWafConfig([]string{}, cdn.WAFMODE_DISABLED, cdn.WAFTYPE_FREE),
			},
			Domains: &[]cdn.Domain{},
		}

		resp := cdn.GetDistributionResponse{
			Distribution: distribution,
		}
		respBytes, _ := json.Marshal(resp)
		w.Write(respBytes)
	}).Methods("GET")
}

// SetupUpdateDistributionHandler adds mock handler for Update API
func (tc *TestContext) SetupUpdateDistributionHandler(callCounter *int) {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("{}"))
	}).Methods("PATCH")
}

// SetupDeleteDistributionHandler adds mock handler for Delete API
func (tc *TestContext) SetupDeleteDistributionHandler(callCounter *int) {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("{}"))
	}).Methods("DELETE")
}

// SetupDeleteDistributionHandlerWithStatus adds mock handler for Delete API that returns a specific status code
func (tc *TestContext) SetupDeleteDistributionHandlerWithStatus(statusCode int, callCounter *int) {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write([]byte("{}"))
	}).Methods("DELETE")
}

// SetupGetDistributionHandlerWithStatus adds mock handler for Get API that returns a specific status code
func (tc *TestContext) SetupGetDistributionHandlerWithStatus(statusCode int) {
	tc.Router.HandleFunc("/v1beta/projects/{projectId}/distributions/{distributionId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write([]byte("{}"))
	}).Methods("GET")
}

// AssertStateFieldEquals checks a single field in the model
func AssertStateFieldEquals(t *testing.T, fieldName string, got, want types.String) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("%s mismatch: got=%s, want=%s", fieldName, got.ValueString(), want.ValueString())
	}
}

// CreateRequest creates a test Create request
func CreateRequest(ctx context.Context, schema resource.SchemaResponse, model Model) resource.CreateRequest {
	req := resource.CreateRequest{}

	// Create plan with model data
	planState := tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), nil),
	}
	diags := planState.Set(ctx, model)
	if diags.HasError() {
		panic(fmt.Sprintf("Failed to set plan: %v", diags))
	}

	req.Plan = tfsdk.Plan{
		Schema: planState.Schema,
		Raw:    planState.Raw,
	}
	return req
}

// CreateResponse creates a test Create response
func CreateResponse(schema resource.SchemaResponse) *resource.CreateResponse {
	resp := &resource.CreateResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(schema.Schema.Type().TerraformType(context.Background()), nil),
	}
	return resp
}

// UpdateRequest creates a test Update request
func UpdateRequest(ctx context.Context, schema resource.SchemaResponse, model Model) resource.UpdateRequest {
	req := resource.UpdateRequest{}

	// Create plan with model data
	planState := tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), nil),
	}
	diags := planState.Set(ctx, model)
	if diags.HasError() {
		panic(fmt.Sprintf("Failed to set plan: %v", diags))
	}

	req.Plan = tfsdk.Plan{
		Schema: planState.Schema,
		Raw:    planState.Raw,
	}
	return req
}

// UpdateResponse creates a test Update response
// Optionally initialize with current state to simulate Terraform framework behavior
func UpdateResponse(ctx context.Context, schema resource.SchemaResponse, currentState *Model) *resource.UpdateResponse {
	resp := &resource.UpdateResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), nil),
	}
	// Initialize with current state to simulate framework behavior
	// When Update errors without calling State.Set(), this state is preserved
	if currentState != nil {
		diags := resp.State.Set(ctx, *currentState)
		if diags.HasError() {
			panic(fmt.Sprintf("Failed to set current state: %v", diags))
		}
	}
	return resp
}

// ReadRequest creates a test Read request
func ReadRequest(ctx context.Context, schema resource.SchemaResponse, model Model) resource.ReadRequest {
	req := resource.ReadRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), nil),
	}
	diags := req.State.Set(ctx, model)
	if diags.HasError() {
		panic(fmt.Sprintf("Failed to set state: %v", diags))
	}
	return req
}

// ReadResponse creates a test Read response
func ReadResponse(ctx context.Context, schema resource.SchemaResponse, currentState *Model) *resource.ReadResponse {
	resp := &resource.ReadResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), nil),
	}
	if currentState != nil {
		diags := resp.State.Set(ctx, *currentState)
		if diags.HasError() {
			panic(fmt.Sprintf("Failed to set current state: %v", diags))
		}
	}
	return resp
}

// DeleteRequest creates a test Delete request
func DeleteRequest(ctx context.Context, schema resource.SchemaResponse, model Model) resource.DeleteRequest {
	req := resource.DeleteRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), nil),
	}
	diags := req.State.Set(ctx, model)
	if diags.HasError() {
		panic(fmt.Sprintf("Failed to set state: %v", diags))
	}
	return req
}

// DeleteResponse creates a test Delete response
// Optionally initialize with current state to simulate Terraform framework behavior
func DeleteResponse(ctx context.Context, schema resource.SchemaResponse, currentState *Model) *resource.DeleteResponse {
	resp := &resource.DeleteResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), nil),
	}
	// Initialize with current state to simulate framework behavior
	// When Delete errors without calling State.RemoveResource(), this state is preserved
	if currentState != nil {
		diags := resp.State.Set(ctx, *currentState)
		if diags.HasError() {
			panic(fmt.Sprintf("Failed to set current state: %v", diags))
		}
	}
	return resp
}

// CreateTestModel creates a Model with all fields properly initialized
func CreateTestModel(projectId, distributionId string, config types.Object) Model {
	var id types.String
	if distributionId != "" {
		id = types.StringValue(fmt.Sprintf("%s,%s", projectId, distributionId))
	} else {
		id = types.StringNull()
	}

	var distIdValue types.String
	if distributionId != "" {
		distIdValue = types.StringValue(distributionId)
	} else {
		distIdValue = types.StringNull()
	}

	return Model{
		ID:             id,
		DistributionId: distIdValue,
		ProjectId:      types.StringValue(projectId),
		Status:         types.StringNull(),
		CreatedAt:      types.StringNull(),
		UpdatedAt:      types.StringNull(),
		Errors:         types.ListNull(types.StringType),
		Domains:        types.ListNull(types.ObjectType{AttrTypes: domainTypes}),
		Config:         config,
	}
}

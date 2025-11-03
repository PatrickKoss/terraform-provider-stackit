package loadbalancer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stackitcloud/stackit-sdk-go/core/config"
	"github.com/stackitcloud/stackit-sdk-go/core/utils"
	"github.com/stackitcloud/stackit-sdk-go/services/loadbalancer"
)

// TestContext holds common test setup
type TestContext struct {
	T          *testing.T
	Server     *httptest.Server
	Client     *loadbalancer.APIClient
	Resource   *loadBalancerResource
	Router     *mux.Router
	Ctx        context.Context
	CancelFunc context.CancelFunc
}

// LoadBalancerResponse represents a test load balancer response
type LoadBalancerResponse struct {
	Name            string
	ProjectId       string
	Region          string
	ExternalAddress string
	PrivateAddress  string
	Status          string
	PlanId          string
}

// NewTestContext creates a new test context with mock server
func NewTestContext(t *testing.T) *TestContext {
	router := mux.NewRouter()
	server := httptest.NewServer(router)

	client, err := loadbalancer.NewAPIClient(
		config.WithEndpoint(server.URL),
		config.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	resource := &loadBalancerResource{
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

// SetupCreateLoadBalancerHandler adds mock handler for Create API
func (tc *TestContext) SetupCreateLoadBalancerHandler(name string, callCounter *int) {
	tc.Router.HandleFunc("/v2/projects/{projectId}/regions/{region}/load-balancers", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)

		resp := loadbalancer.LoadBalancer{
			Name: utils.Ptr(name),
		}
		respBytes, _ := json.Marshal(resp)
		w.Write(respBytes)
	}).Methods("POST")
}

// SetupGetLoadBalancerHandler adds mock handler for Get API
func (tc *TestContext) SetupGetLoadBalancerHandler(lbResp LoadBalancerResponse) {
	tc.Router.HandleFunc("/v2/projects/{projectId}/regions/{region}/load-balancers/{name}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Return minimal JSON with STATUS_READY so wait handler completes
		// Default status to STATUS_READY if not provided
		status := lbResp.Status
		if status == "" {
			status = "STATUS_READY"
		}

		jsonResp := fmt.Sprintf(`{
			"name": "%s",
			"externalAddress": "%s",
			"privateAddress": "%s",
			"status": "%s"
		}`, lbResp.Name, lbResp.ExternalAddress, lbResp.PrivateAddress, status)
		w.Write([]byte(jsonResp))
	}).Methods("GET")
}

// SetupUpdateTargetPoolHandler adds mock handler for Update API
func (tc *TestContext) SetupUpdateTargetPoolHandler(callCounter *int) {
	tc.Router.HandleFunc("/v2/projects/{projectId}/regions/{region}/load-balancers/{name}/target-pools/{targetPoolName}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("{}"))
	}).Methods("PUT")
}

// SetupDeleteLoadBalancerHandler adds mock handler for Delete API
func (tc *TestContext) SetupDeleteLoadBalancerHandler(callCounter *int) {
	tc.Router.HandleFunc("/v2/projects/{projectId}/regions/{region}/load-balancers/{name}", func(w http.ResponseWriter, r *http.Request) {
		if callCounter != nil {
			*callCounter++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("{}"))
	}).Methods("DELETE")
}

// AssertStateFieldEquals checks a single field in the model
func AssertStateFieldEquals(t *testing.T, fieldName string, got, want types.String) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("%s mismatch: got=%s, want=%s", fieldName, got.ValueString(), want.ValueString())
	}
}

// CreateTestModel creates a test model with basic configuration
func CreateTestModel(projectId, region, name, externalAddress string) Model {
	ctx := context.Background()

	// Create minimal target
	targetObj, _ := types.ObjectValue(targetTypes, map[string]attr.Value{
		"display_name": types.StringValue("target1"),
		"ip":           types.StringValue("192.168.1.10"),
	})
	targetsValue, _ := types.ListValue(types.ObjectType{AttrTypes: targetTypes}, []attr.Value{targetObj})

	// Create minimal target pool
	targetPoolObj, _ := types.ObjectValue(targetPoolTypes, map[string]attr.Value{
		"name":                 types.StringValue("pool1"),
		"target_port":          types.Int64Value(8080),
		"targets":              targetsValue,
		"active_health_check":  types.ObjectNull(activeHealthCheckTypes),
		"session_persistence":  types.ObjectNull(sessionPersistenceTypes),
	})
	targetPoolsValue, _ := types.ListValue(types.ObjectType{AttrTypes: targetPoolTypes}, []attr.Value{targetPoolObj})

	// Create minimal listener
	listenerObj, _ := types.ObjectValue(listenerTypes, map[string]attr.Value{
		"port":                   types.Int64Value(80),
		"protocol":               types.StringValue("PROTOCOL_TCP"),
		"target_pool":            types.StringValue("pool1"),
		"display_name":           types.StringNull(),
		"server_name_indicators": types.ListNull(types.ObjectType{AttrTypes: serverNameIndicatorTypes}),
		"tcp":                    types.ObjectNull(tcpTypes),
		"udp":                    types.ObjectNull(udpTypes),
	})
	listenersValue, _ := types.ListValue(types.ObjectType{AttrTypes: listenerTypes}, []attr.Value{listenerObj})

	// Create minimal network
	networkObj, _ := types.ObjectValue(networkTypes, map[string]attr.Value{
		"network_id": types.StringValue("network-123"),
		"role":       types.StringValue("ROLE_LISTENERS_AND_TARGETS"),
	})
	networksValue, _ := types.ListValue(types.ObjectType{AttrTypes: networkTypes}, []attr.Value{networkObj})

	model := Model{
		ProjectId:                      types.StringValue(projectId),
		Name:                           types.StringValue(name),
		Listeners:                      listenersValue,
		Networks:                       networksValue,
		TargetPools:                    targetPoolsValue,
		ExternalAddress:                types.StringValue(externalAddress),
		Region:                         types.StringValue(region),
		Id:                             types.StringNull(),
		PlanId:                         types.StringNull(),
		Options:                        types.ObjectNull(optionsTypes),
		PrivateAddress:                 types.StringNull(),
		SecurityGroupId:                types.StringNull(),
		DisableSecurityGroupAssignment: types.BoolValue(false),
	}

	_ = ctx // unused but kept for consistency

	return model
}

// Request/Response builders
func CreateRequest(ctx context.Context, schema resource.SchemaResponse, model Model) resource.CreateRequest {
	req := resource.CreateRequest{}
	req.Plan = tfsdk.Plan{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.Plan.Set(ctx, model)
	return req
}

func CreateResponse(schema resource.SchemaResponse) *resource.CreateResponse {
	resp := &resource.CreateResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	return resp
}

func UpdateRequest(ctx context.Context, schema resource.SchemaResponse, model Model) resource.UpdateRequest {
	req := resource.UpdateRequest{}
	req.Plan = tfsdk.Plan{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.Plan.Set(ctx, model)
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, model)
	return req
}

func UpdateResponse(ctx context.Context, schema resource.SchemaResponse, currentState *Model) *resource.UpdateResponse {
	resp := &resource.UpdateResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	if currentState != nil {
		resp.State.Set(ctx, *currentState)
	}
	return resp
}

func DeleteRequest(ctx context.Context, schema resource.SchemaResponse, model Model) resource.DeleteRequest {
	req := resource.DeleteRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, model)
	return req
}

func DeleteResponse(ctx context.Context, schema resource.SchemaResponse, currentState *Model) *resource.DeleteResponse {
	resp := &resource.DeleteResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	if currentState != nil {
		resp.State.Set(ctx, *currentState)
	}
	return resp
}

func ReadRequest(ctx context.Context, schema resource.SchemaResponse, model Model) resource.ReadRequest {
	req := resource.ReadRequest{}
	req.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	req.State.Set(ctx, model)
	return req
}

func ReadResponse(ctx context.Context, schema resource.SchemaResponse, currentState *Model) *resource.ReadResponse {
	resp := &resource.ReadResponse{}
	resp.State = tfsdk.State{
		Schema: schema.Schema,
		Raw:    tftypes.NewValue(tftypes.DynamicPseudoType, nil),
	}
	if currentState != nil {
		resp.State.Set(ctx, *currentState)
	}
	return resp
}

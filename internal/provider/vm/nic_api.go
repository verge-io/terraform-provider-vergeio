// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// NIC Resource Models.
type nicResourceModel struct {
	Id              types.String `tfsdk:"id"`
	Machine         types.Int32  `tfsdk:"machine"`
	Name            types.String `tfsdk:"name"`
	Description     types.String `tfsdk:"description"`
	Interface       types.String `tfsdk:"interface"`
	Driver          types.String `tfsdk:"driver"`
	Model           types.String `tfsdk:"model"`
	Vendor          types.String `tfsdk:"vendor"`
	Port            types.Int32  `tfsdk:"port"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	VNET            types.Int32  `tfsdk:"vnet"`
	MAC             types.String `tfsdk:"macaddress"`
	IPAddress       types.String `tfsdk:"ipaddress"`
	AssignIPAddress types.Bool   `tfsdk:"assign_ipaddress"`
	Asset           types.String `tfsdk:"asset"`
}

type nicAPIResourceModel struct {
	Id              string `json:"id,omitempty"`
	Machine         int32  `json:"machine,omitempty"`
	Name            string `json:"name,omitempty"`
	Description     string `json:"description,omitempty"`
	Interface       string `json:"interface,omitempty"`
	Driver          string `json:"driver,omitempty"`
	Model           string `json:"model,omitempty"`
	Vendor          string `json:"vendor,omitempty"`
	Port            int32  `json:"port,omitempty"`
	Enabled         bool   `json:"enabled"`
	VNET            int32  `json:"vnet,omitempty"`
	MAC             string `json:"macaddress,omitempty"`
	IPAddress       string `json:"ipaddress,omitempty"`
	AssignIPAddress bool   `json:"assign_ipaddress,omitempty"`
	Asset           string `json:"asset,omitempty"`
}

// to get the power status.
type nicAPIPowerStatus struct {
	PowerState string `json:"powerstate,omitempty"`
}

// To assign an IP.
type nicIPAPIResourceModel struct {
	VNET int32  `json:"vnet,omitempty"`
	MAC  string `json:"mac,omitempty"`
	Type string `json:"type,omitempty"`
}

// NIC Endpoints.
const (
	NICEndpoint = vergeio.APIEndpoint + "/machine_nics"
	IPEndpoint  = vergeio.APIEndpoint + "/vnet_addresses"
)

var _ vergeio.IClient = &NICApi{}

func NewNICApi(c *vergeio.Client) *NICApi {
	sdk, _ := vergeos.NewClient(
		vergeos.WithBaseURL(vergeio.EnsureHTTPSPrefix(c.Host)),
		vergeos.WithCredentials(c.Username, c.Password),
		vergeos.WithInsecureTLS(c.Insecure),
	)
	return &NICApi{
		name:   "NIC Api",
		client: c,
		sdk:    sdk,
	}
}

type NICApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
}

func (nc *NICApi) Name() string {
	return nc.name
}

// Create the NIC in the API.
func (nc *NICApi) createNIC(ctx context.Context, data *nicResourceModel) error {

	// Build from known plan values only. Null and unknown attributes are left
	// out so a create does not send the zero value (enabled=false) over a
	// platform default. See nicCreatePayload.
	apiData := nicCreatePayload(data)

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for NIC")
	}

	// Call the API and check the response
	apiResp, err := nc.client.Post(NICEndpoint, encodedBuffer)
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 201 {
		return fmt.Errorf("missing response from the API %d", apiResp.StatusCode)
	}

	// Decode the API response
	var nicAPIResp vergeio.VergeResponse
	if err := json.NewDecoder(apiResp.Body).Decode(&nicAPIResp); err != nil {
		return errors.New("invalid format received for creating the NIC")
	}

	// save into the Terraform state.
	data.Id = types.StringValue(nicAPIResp.Key)
	tflog.Debug(ctx, fmt.Sprintf("Created a nic with Id %v", data.Id.ValueString()))

	// Read it back from the API to get all the fields
	if readError := nc.readNIC(ctx, data); readError != nil {
		return errors.New("Error reading the nic: " + readError.Error())
	}

	// After creating the nic, assign an IP to it.
	if data.AssignIPAddress.ValueBool() {
		if ipError := nc.assignIP(data); ipError != nil {
			return errors.New("Error assigning an IP to the nic: " + ipError.Error())
		}
	} else {
		data.IPAddress = types.StringValue("N/A")
	}

	return nil
}

// Assign an IP to the NIC in the API.
func (nc *NICApi) assignIP(data *nicResourceModel) error {

	tflog.Debug(context.Background(), fmt.Sprintf("Assigning an IP to the nic %v", data.Id.ValueString()))
	tflog.Debug(context.Background(), fmt.Sprintf("MAC of the nic %v", data.MAC.ValueString()))

	apiData := nicIPAPIResourceModel{
		VNET: data.VNET.ValueInt32(),
		MAC:  data.MAC.ValueString(),
		Type: "static",
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for the IP")
	}

	// Call the API and check the response
	apiResp, err := nc.client.Post(IPEndpoint, encodedBuffer)
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 201 {
		return fmt.Errorf("missing response from the API %d", apiResp.StatusCode)
	}

	// Decode the API response
	var nicAPIResp vergeio.VergeResponse
	if err := json.NewDecoder(apiResp.Body).Decode(&nicAPIResp); err != nil {
		return errors.New("invalid format received for creating the NIC")
	} else {
		data.IPAddress = types.StringValue(nicAPIResp.Response)
	}

	return nil
}

// Update the NIC in the API.
func (nc *NICApi) updateNIC(ctx context.Context, planData *nicResourceModel, stateData *nicResourceModel) error {

	// Prepare the API data packet from the plan
	apiData := nicAPIResourceModel{
		Machine:     vergeio.Int32ToNil(planData.Machine, stateData.Machine, 0),
		Name:        vergeio.StringToNil(planData.Name, stateData.Name, ""),
		Description: vergeio.StringToNil(planData.Description, stateData.Description, ""),
		Interface:   vergeio.StringToNil(planData.Interface, stateData.Interface, ""),
		Driver:      vergeio.StringToNil(planData.Driver, stateData.Driver, ""),
		Model:       vergeio.StringToNil(planData.Model, stateData.Model, ""),
		Vendor:      vergeio.StringToNil(planData.Vendor, stateData.Vendor, ""),
		Port:        vergeio.Int32ToNil(planData.Port, stateData.Port, 0),
		Enabled:     vergeio.BoolToNil(planData.Enabled, stateData.Enabled, false),
		VNET:        vergeio.Int32ToNil(planData.VNET, stateData.VNET, 0),
		MAC:         vergeio.StringToNil(planData.MAC, stateData.MAC, ""),
		Asset:       vergeio.StringToNil(planData.Asset, stateData.Asset, ""),
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for the NIC")
	}

	// Call the API and check the response
	apiResp, err := nc.client.Put(fmt.Sprintf("%s/%s",
		NICEndpoint,
		url.PathEscape(stateData.Id.ValueString()),
	), encodedBuffer)

	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("missing response from the API %d", apiResp.StatusCode)
	}

	defer apiResp.Body.Close()

	// Read data into the model to get all the attributes
	if readDataError := nc.readNIC(ctx, stateData); readDataError != nil {
		return fmt.Errorf("error Fetching Data %v", readDataError)
	}

	tflog.Debug(ctx, fmt.Sprintf("Updated nic %v", stateData.Id.ValueString()))

	return nil
}

// Read the NIC from the API.
func (nc *NICApi) readNIC(ctx context.Context, data *nicResourceModel) error {

	tflog.Debug(ctx, "Reading the nic data")

	// Call the Get API with the nic id and get the fields we need
	// most fields are not returned by default
	apiResp, err := nc.client.Get(fmt.Sprintf("%s/%s",
		NICEndpoint,
		url.PathEscape(data.Id.ValueString()),
	), &vergeio.Options{Fields: "machine,name,description,interface,driver,model,vendor,port,enabled,vnet,macaddress,asset"})

	// error checking
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("missing response from API %d", apiResp.StatusCode)
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the NIC from the API %v", apiResp.Body))

	// Decode the API response
	var nicAPIResp nicAPIResourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&nicAPIResp); err != nil {
		return errors.New("invalid format received for Item")
	}

	// save into the resource model
	data.Machine = types.Int32Value(nicAPIResp.Machine)
	data.Name = types.StringValue(nicAPIResp.Name)
	data.Description = types.StringValue(nicAPIResp.Description)
	data.Interface = types.StringValue(nicAPIResp.Interface)
	data.Driver = types.StringValue(nicAPIResp.Driver)
	data.Model = types.StringValue(nicAPIResp.Model)
	data.Vendor = types.StringValue(nicAPIResp.Vendor)
	data.Port = types.Int32Value(nicAPIResp.Port)
	data.Enabled = types.BoolValue(nicAPIResp.Enabled)
	data.VNET = types.Int32Value(nicAPIResp.VNET)
	data.MAC = types.StringValue(nicAPIResp.MAC)
	data.Asset = types.StringValue(nicAPIResp.Asset)

	tflog.Debug(ctx, "NIC Data was successfully read from the API")

	return nil
}

// readNICsByMachine lists machine_nics for a VM machine and reads each row.
// machineID is the VM's machine id, not the VM row key. Order is name, then
// id, so two reads return the same block order.
func (na *NICApi) readNICsByMachine(ctx context.Context, machineID int32) ([]*nicResourceModel, error) {
	nics, err := na.sdk.VMNICs.List(ctx, int(machineID))
	if err != nil {
		return nil, fmt.Errorf("listing NICs for machine %d: %w", machineID, err)
	}
	sortNICsForState(nics)
	if len(nics) == 0 {
		return nil, nil
	}

	models := make([]*nicResourceModel, 0, len(nics))
	for _, nic := range nics {
		model := &nicResourceModel{Id: types.StringValue(strconv.Itoa(nic.ID.Int()))}
		if err := na.readNIC(ctx, model); err != nil {
			return nil, fmt.Errorf("reading NIC %d: %w", nic.ID.Int(), err)
		}
		// createNIC records "N/A" when no address is assigned. Match that so
		// an import compares equal to the state left by create.
		model.IPAddress = nicIPFromAPI(nic.IPAddress)
		models = append(models, model)
	}
	return models, nil
}

// sortNICsForState orders NICs the way nested blocks are stored.
func sortNICsForState(nics []vergeos.VMNIC) {
	sort.SliceStable(nics, func(i, j int) bool {
		if nics[i].Name != nics[j].Name {
			return nics[i].Name < nics[j].Name
		}
		return nics[i].ID.Int() < nics[j].ID.Int()
	})
}

// nicIPFromAPI maps the API address onto the value create stores.
// An unassigned NIC is recorded as "N/A".
func nicIPFromAPI(ip string) types.String {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return types.StringValue("N/A")
	}
	return types.StringValue(ip)
}

// preserveNICConfigFields copies attributes that are not on machine_nics.
// assign_ipaddress is a create-time flag, so a refresh keeps the configured value.
func preserveNICConfigFields(prior, current []*nicResourceModel) {
	byID := make(map[string]*nicResourceModel, len(prior))
	for _, nic := range prior {
		if nic == nil || nic.Id.IsNull() || nic.Id.ValueString() == "" {
			continue
		}
		byID[nic.Id.ValueString()] = nic
	}
	for _, nic := range current {
		if nic == nil || nic.Id.IsNull() {
			continue
		}
		old, ok := byID[nic.Id.ValueString()]
		if !ok {
			continue
		}
		nic.AssignIPAddress = old.AssignIPAddress
	}
}

// Delete the NIC from the API.
func (na *NICApi) deleteNIC(ctx context.Context, data *nicResourceModel, vmId types.String) error {

	tflog.Debug(ctx, "Deleting the nic")

	// VM API for hotplugging via VM Actions
	vmApi := NewVMApi(na.client)

	var powerState string = ""

	// Call the API to check if the nic is in a power state that can be deleted
	if err := na.checkNICPowerState(ctx, data.Id.ValueString(), &powerState); err != nil {
		return fmt.Errorf("error checking NIC power state %v", err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Current NIC power state is %v", powerState))

	// Call the API to check if the vm is in a power state that can be deleted
	for strings.ToLower(powerState) != "down" {
		Retries := 1

		// Power the vm off
		if err := vmApi.hotplugNIC(ctx, data.Id.ValueString(), vmId); err != nil {
			return fmt.Errorf("failed to hotplug NIC: %v", err)
		}

		// Wait for a short period to allow the kill operation to complete
		time.Sleep(2 * time.Second)

		// Call the API to check if the nic is in a power state that can be deleted
		if err := na.checkNICPowerState(ctx, data.Id.ValueString(), &powerState); err != nil {
			return fmt.Errorf("error checking NIC power state %v", err)
		}

		Retries += 1

		// We are only going to retry 5 times before giving up
		if Retries > 5 {
			return fmt.Errorf("failed to kill NIC before deletion after %d retries", Retries)
		}
		continue
	}

	// Call the API
	_, apiErr := na.client.Delete(NICEndpoint + "/" + data.Id.ValueString())

	if apiErr != nil {
		return errors.New("Error deleting the NIC: " + apiErr.Error())
	}

	tflog.Debug(ctx, "NIC was successfully deleted")

	return nil
}

// Update, Create, Delete the NIC in the API.
// This method is called from VM update method.
func (na *NICApi) syncNICs(ctx context.Context, planData *[]*nicResourceModel, stateData *[]*nicResourceModel, machine types.Int32, vmId types.String) error {

	tflog.Debug(ctx, "Syncing NICs: Starting deletion")

	var stateToBeDeleted []string // List of disks to be deleted

	// Delete the nics that are not in the plan
	for _, state := range *stateData {
		found := false
		for _, plan := range *planData {
			if plan.Name.ValueString() == state.Name.ValueString() {
				found = true
				break
			}
		}
		if !found {
			if err := na.deleteNIC(ctx, state, vmId); err != nil {
				return fmt.Errorf("failed to delete nic: %v", err)
			}
			stateToBeDeleted = append(stateToBeDeleted, state.Name.ValueString())
		}
	}

	// Delete the nic that are not in the state
	for _, name := range stateToBeDeleted {
		for i, state := range *stateData {
			if state.Name.ValueString() == name {
				*stateData = append((*stateData)[:i], (*stateData)[i+1:]...)
				break
			}
		}
	}

	tflog.Debug(ctx, "Syncing NICs: Starting updation")

	// Compare the plan data with the state data and update
	for _, plan := range *planData {
		for _, state := range *stateData {
			if plan.Name.ValueString() == state.Name.ValueString() {
				if plan.Description.ValueString() != state.Description.ValueString() ||
					plan.Interface.ValueString() != state.Interface.ValueString() ||
					plan.Driver.ValueString() != state.Driver.ValueString() ||
					plan.Model.ValueString() != state.Model.ValueString() ||
					plan.Vendor.ValueString() != state.Vendor.ValueString() ||
					plan.Port.ValueInt32() != state.Port.ValueInt32() ||
					plan.VNET.ValueInt32() != state.VNET.ValueInt32() ||
					plan.MAC.ValueString() != state.MAC.ValueString() ||
					plan.Asset.ValueString() != state.Asset.ValueString() ||
					plan.Enabled.ValueBool() != state.Enabled.ValueBool() {
					if err := na.updateNIC(ctx, plan, state); err != nil {
						return fmt.Errorf("failed to update nic: %v", err)
					}
				}
			}
		}
	}

	tflog.Debug(ctx, "Syncing NICs: Starting insertion")

	// Create the nics that are not in the state
	for i, plan := range *planData {
		found := false
		for _, state := range *stateData {
			if plan.Name.ValueString() == state.Name.ValueString() {
				found = true
				break
			}
		}
		if !found {
			plan.Machine = machine
			if err := na.createNIC(ctx, plan); err != nil {
				return fmt.Errorf("failed to create nic: %v", err)
			}
			if err := na.readNIC(ctx, plan); err != nil {
				return fmt.Errorf("failed to read disk during the sync: %v", err)
			}
			// insert plan in the stateData in a particular order
			na.insertNICInState(i, plan, stateData) // insert nic in the stateData in a particular order na.insertDiskInState(i, plan, stateData)
		}
	}
	return nil
}

// Manually insert plan in the stateData in a particular order.
func (na *NICApi) insertNICInState(insertAt int, planData *nicResourceModel, stateData *[]*nicResourceModel) {
	// If it's at the end ten just append
	if insertAt >= len(*stateData) {
		*stateData = append(*stateData, planData)
	} else {
		// insert plan in the stateData in a particular order
		for i := range *stateData {
			if i == insertAt {
				*stateData = append((*stateData)[:i], append([]*nicResourceModel{planData}, (*stateData)[i:]...)...)
				break
			}
		}
	}
}

// to get the power status of the nic.
func (na *NICApi) checkNICPowerState(ctx context.Context, key string, powerState *string) error {

	tflog.Debug(ctx, fmt.Sprintf("Checking the power state of the NIC %v", key))

	// Call the API to get the NIC status
	apiResp, err := na.client.Get(fmt.Sprintf("%s/%s",
		NICEndpoint,
		url.PathEscape(key)),
		&vergeio.Options{
			Fields: "status#status as powerState"})

	// error checking
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("missing response from API %d", apiResp.StatusCode)
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the powerstatus of the NIC %v", apiResp.Body))

	// Decode the API response
	var nicAPIResp nicAPIPowerStatus
	if err := json.NewDecoder(apiResp.Body).Decode(&nicAPIResp); err != nil {
		return errors.New("invalid format received for nic power state call")
	}

	// save into the resource model
	*powerState = nicAPIResp.PowerState

	tflog.Debug(ctx, fmt.Sprintf("NIC status read from API: %v", nicAPIResp.PowerState))

	return nil
}

// nicCreatePayload is the JSON body for NIC create.
// Unset (null or unknown) attributes are omitted. An explicit false is sent.
func nicCreatePayload(data *nicResourceModel) map[string]any {
	payload := map[string]any{}
	putKnownString(payload, "name", data.Name)
	putKnownString(payload, "description", data.Description)
	putKnownString(payload, "interface", data.Interface)
	putKnownString(payload, "driver", data.Driver)
	putKnownString(payload, "model", data.Model)
	putKnownString(payload, "vendor", data.Vendor)
	putKnownString(payload, "macaddress", data.MAC)
	putKnownString(payload, "asset", data.Asset)
	putKnownInt32(payload, "machine", data.Machine)
	putKnownInt32(payload, "port", data.Port)
	putKnownInt32(payload, "vnet", data.VNET)
	putKnownBool(payload, "enabled", data.Enabled)
	return payload
}

func putKnownString(payload map[string]any, key string, value types.String) {
	if value.IsNull() || value.IsUnknown() || value.ValueString() == "" {
		return
	}
	payload[key] = value.ValueString()
}

func putKnownInt32(payload map[string]any, key string, value types.Int32) {
	if value.IsNull() || value.IsUnknown() || value.ValueInt32() == 0 {
		return
	}
	payload[key] = value.ValueInt32()
}

func putKnownBool(payload map[string]any, key string, value types.Bool) {
	if value.IsNull() || value.IsUnknown() {
		return
	}
	payload[key] = value.ValueBool()
}

func (va *VMApi) hotplugNIC(ctx context.Context, nicId string, vmId types.String) error {

	tflog.Debug(ctx, fmt.Sprintf("Calling the VMActions API for the NIC %v", nicId))

	// Create the action payload according to vm_actions schema
	params := VMActionParams{
		Device: nicId,
		Unplug: true,
	}
	intVMId, err := strconv.Atoi(vmId.ValueString())
	if err != nil {
		return fmt.Errorf("invalid VM ID format: %v", err)
	}
	actionPayload := VMAction{
		VM:     int32(intVMId),
		Action: "hotplugnic",
		Params: params,
	}
	tflog.Debug(ctx, fmt.Sprintf("Calling the hotplug NIC API with the action payload %v", actionPayload))
	bytedata, err := json.Marshal(actionPayload)
	if err != nil {
		return err
	}

	// Send the kill action request to the vnet_actions endpoint
	req, err := va.client.Post(VMActionEndpoint, bytes.NewBuffer(bytedata))
	if err != nil {
		return err
	}
	if req.StatusCode != 201 {
		return fmt.Errorf("failed to kill NIC: status code %v", req.StatusCode)
	}

	return nil
}

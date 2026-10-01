// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"terraform-provider-vergeio/internal/client"

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

// nicAPIResourceModel is the NIC update body and the decoded GET.
// Enabled is a pointer with omitempty: an unset NIC omits enabled, and an
// explicit false is sent. A plain bool with omitempty cannot do both.
type nicAPIResourceModel struct {
	Id              *string `json:"id,omitempty"`
	Machine         *int32  `json:"machine,omitempty"`
	Name            *string `json:"name,omitempty"`
	Description     *string `json:"description,omitempty"`
	Interface       *string `json:"interface,omitempty"`
	Driver          *string `json:"driver,omitempty"`
	Model           *string `json:"model,omitempty"`
	Vendor          *string `json:"vendor,omitempty"`
	Port            *int32  `json:"port,omitempty"`
	Enabled         *bool   `json:"enabled,omitempty"`
	VNET            *int32  `json:"vnet,omitempty"`
	MAC             *string `json:"macaddress,omitempty"`
	IPAddress       *string `json:"ipaddress,omitempty"`
	AssignIPAddress *bool   `json:"assign_ipaddress,omitempty"`
	Asset           *string `json:"asset,omitempty"`
}

// to get the power status.
type nicAPIPowerStatus struct {
	PowerState string `json:"powerstate,omitempty"`
}

// nicIPAPIResourceModel is the body for POST /vnet_addresses.
// IP is the address to reserve. An empty IP is omitted so VergeOS
// assigns the next free address.
type nicIPAPIResourceModel struct {
	VNET int32  `json:"vnet,omitempty"`
	MAC  string `json:"mac,omitempty"`
	Type string `json:"type,omitempty"`
	IP   string `json:"ip,omitempty"`
}

// NIC Endpoints.
const (
	NICEndpoint = vergeio.NICEndpoint
	IPEndpoint  = vergeio.IPEndpoint
)

var _ vergeio.IClient = &NICApi{}

func NewNICApi(c *vergeio.Client) (*NICApi, error) {
	sdk, err := c.NewVergeosClient()
	if err != nil {
		return nil, err
	}
	return &NICApi{
		name:   "NIC Api",
		client: c,
		sdk:    sdk,
	}, nil
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
	apiResp, err := nc.client.Post(ctx, NICEndpoint, encodedBuffer)
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
		if ipError := nc.assignIP(ctx, data); ipError != nil {
			return errors.New("Error assigning an IP to the nic: " + ipError.Error())
		}
	} else {
		// No address was reserved. Null matches an unset ipaddress.
		// A placeholder such as "N/A" does not.
		data.IPAddress = types.StringNull()
	}

	return nil
}

// Assign an IP to the NIC in the API.
func (nc *NICApi) assignIP(ctx context.Context, data *nicResourceModel) error {

	tflog.Debug(ctx, fmt.Sprintf("Assigning an IP to the nic %v", data.Id.ValueString()))
	tflog.Debug(ctx, fmt.Sprintf("MAC of the nic %v", data.MAC.ValueString()))

	apiData := nicIPAssignPayload(data)

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for the IP")
	}

	// Call the API and check the response
	apiResp, err := nc.client.Post(ctx, IPEndpoint, encodedBuffer)
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

	apiData := nicUpdatePayload(planData, stateData)

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for the NIC")
	}

	// Call the API and check the response
	apiResp, err := nc.client.Put(ctx, vergeio.ObjectPath(NICEndpoint, stateData.Id.ValueString()), encodedBuffer)

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
	apiResp, err := nc.client.Get(ctx, vergeio.ObjectPath(NICEndpoint, data.Id.ValueString()), &vergeio.Options{Fields: "machine,name,description,interface,driver,model,vendor,port,enabled,vnet,macaddress,asset"})

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
	data.Machine = types.Int32Value(vergeio.Int32Or(nicAPIResp.Machine, 0))
	data.Name = types.StringValue(vergeio.StringOr(nicAPIResp.Name, ""))
	data.Description = types.StringValue(vergeio.StringOr(nicAPIResp.Description, ""))
	data.Interface = types.StringValue(vergeio.StringOr(nicAPIResp.Interface, ""))
	data.Driver = types.StringValue(vergeio.StringOr(nicAPIResp.Driver, ""))
	data.Model = types.StringValue(vergeio.StringOr(nicAPIResp.Model, ""))
	data.Vendor = types.StringValue(vergeio.StringOr(nicAPIResp.Vendor, ""))
	data.Port = types.Int32Value(vergeio.Int32Or(nicAPIResp.Port, 0))
	data.Enabled = types.BoolValue(vergeio.BoolOr(nicAPIResp.Enabled, false))
	data.VNET = types.Int32Value(vergeio.Int32Or(nicAPIResp.VNET, 0))
	data.MAC = types.StringValue(vergeio.StringOr(nicAPIResp.MAC, ""))
	data.Asset = types.StringValue(vergeio.StringOr(nicAPIResp.Asset, ""))

	tflog.Debug(ctx, "NIC Data was successfully read from the API")

	return nil
}

// readNICsByMachine lists machine_nics for a VM machine and reads each row.
// machineID is the VM's machine id, not the VM row key. Order is name, then
// id, so two reads return the same block order.
func (na *NICApi) readNICsByMachine(ctx context.Context, machineID int32) ([]*nicResourceModel, error) {
	// VMNICs.List takes a VM $key and resolves the machine. Callers have the
	// machine id, which is what govergeos v0.3.0 filtered on directly.
	apiResp, err := na.client.Get(ctx, NICEndpoint, &vergeio.Options{
		Fields: "$key,name",
		Filter: fmt.Sprintf("machine eq %d", machineID),
	})
	if err != nil {
		return nil, fmt.Errorf("listing NICs for machine %d: %w", machineID, err)
	}
	if apiResp == nil {
		return nil, fmt.Errorf("listing NICs for machine %d: missing response", machineID)
	}
	defer apiResp.Body.Close()
	if apiResp.StatusCode != 200 {
		return nil, fmt.Errorf("listing NICs for machine %d: status %d", machineID, apiResp.StatusCode)
	}
	var nics []vergeos.VMNIC
	if err := json.NewDecoder(apiResp.Body).Decode(&nics); err != nil {
		return nil, fmt.Errorf("listing NICs for machine %d: %w", machineID, err)
	}
	sortNICsForState(nics)
	if len(nics) == 0 {
		return nil, nil
	}

	models := make([]*nicResourceModel, 0, len(nics))
	for _, nic := range nics {
		model := &nicResourceModel{Id: types.StringValue(strconv.Itoa(nic.Key.Int()))}
		if err := na.readNIC(ctx, model); err != nil {
			return nil, fmt.Errorf("reading NIC %d: %w", nic.Key.Int(), err)
		}
		// createNIC leaves ipaddress null when no address is assigned. Match
		// that so an import compares equal to the state left by create.
		model.IPAddress = nicIPFromAPI(nic.IPAddress)
		models = append(models, model)
	}
	return models, nil
}

// findNICByName returns the NIC with this name on the machine.
// A nil NIC and a nil error means the name is not in use. Two NICs with
// the same name is an error so create does not guess which one to adopt.
func (na *NICApi) findNICByName(ctx context.Context, machineID int32, name string) (*nicResourceModel, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("NIC name is empty")
	}
	nics, err := na.readNICsByMachine(ctx, machineID)
	if err != nil {
		return nil, err
	}
	var found *nicResourceModel
	for _, nic := range nics {
		if nic == nil || strings.TrimSpace(nic.Name.ValueString()) != name {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("more than one NIC named %q on machine %d", name, machineID)
		}
		found = nic
	}
	return found, nil
}

// sortNICsForState orders NICs the way nested blocks are stored.
func sortNICsForState(nics []vergeos.VMNIC) {
	sort.SliceStable(nics, func(i, j int) bool {
		if nics[i].Name != nics[j].Name {
			return nics[i].Name < nics[j].Name
		}
		return nics[i].Key.Int() < nics[j].Key.Int()
	})
}

// nicIPFromAPI maps the API address onto the value create stores.
// An unassigned NIC is null, not a placeholder string.
func nicIPFromAPI(ip string) types.String {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return types.StringNull()
	}
	return types.StringValue(ip)
}

// nicIPAssignPayload is the JSON body for POST /vnet_addresses.
// A configured ipaddress is sent as ip so VergeOS reserves that address.
// An unset or blank address is omitted, and VergeOS assigns the next free one.
func nicIPAssignPayload(data *nicResourceModel) nicIPAPIResourceModel {
	payload := nicIPAPIResourceModel{
		Type: "static",
	}
	if data == nil {
		return payload
	}
	payload.VNET = data.VNET.ValueInt32()
	payload.MAC = data.MAC.ValueString()
	if ip := requestedNICIP(data.IPAddress); ip != "" {
		payload.IP = ip
	}
	return payload
}

// requestedNICIP is the address from configuration.
// Null, unknown, and blank values are not a request.
func requestedNICIP(ip types.String) string {
	if ip.IsNull() || ip.IsUnknown() {
		return ""
	}
	return strings.TrimSpace(ip.ValueString())
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

// deleteNIC unplugs a NIC that is still attached, waits until it is down,
// and then deletes it. The unplug action is sent once. Sending it again
// while the guest is still releasing the NIC returns 422.
//
// Terraform destroys vergeio_vm_nic before vergeio_vm. A guest that ignores
// the unplug, such as firmware with no OS, leaves the NIC up. Failing that
// delete skips the VM, so the VM and its network stay behind. That guest is
// powered off with kill and the NIC is deleted. A guest that enters hotplug
// is left running and waited on for deviceDetachTimeout. A stopped VM has
// no guest to release the NIC, so the NIC is deleted without an unplug.
func (na *NICApi) deleteNIC(ctx context.Context, data *nicResourceModel, vmId types.String) error {

	tflog.Debug(ctx, "Deleting the nic")

	id := data.Id.ValueString()
	var powerState string
	if err := na.checkNICPowerState(ctx, id, &powerState); err != nil {
		return fmt.Errorf("error checking NIC power state %v", err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Current NIC power state is %v", powerState))

	name := ""
	if !data.Name.IsNull() && !data.Name.IsUnknown() {
		name = data.Name.ValueString()
	}

	if strings.EqualFold(strings.TrimSpace(powerState), "down") {
		return na.removeNIC(ctx, id)
	}

	running, err := readVMPowerState(ctx, na.client, vmId)
	if err != nil {
		return fmt.Errorf("checking VM power state before deleting NIC: %w", err)
	}
	if !running {
		tflog.Debug(ctx, fmt.Sprintf("VM %s is stopped; deleting NIC %s without waiting for a guest unplug", vmId.ValueString(), detachLabel(name, id, "id")))
		return na.removeStoppedVMNIC(ctx, id, name)
	}

	if err := sendUnplug(ctx, powerState, func() error {
		vmAPI, err := NewVMApi(na.client)
		if err != nil {
			return err
		}
		return vmAPI.hotplugNIC(ctx, id, vmId, true)
	}); err != nil {
		return fmt.Errorf("failed to unplug NIC: %v", err)
	}

	if err := waitUntilNICDown(ctx, func() (string, error) {
		var current string
		readErr := na.checkNICPowerState(ctx, id, &current)
		return current, readErr
	}, name, id); err != nil {
		if !errors.Is(err, errNICUnplugIgnored) {
			return err
		}
		tflog.Debug(ctx, fmt.Sprintf("NIC %s stayed up after unplug; killing VM %s so the NIC can be deleted", detachLabel(name, id, "id"), vmId.ValueString()))
		if err := na.killVMForNICDelete(ctx, vmId); err != nil {
			return err
		}
		return na.removeStoppedVMNIC(ctx, id, name)
	}

	return na.removeNIC(ctx, id)
}

// killVMForNICDelete powers the VM off so a NIC the guest will not release
// can be deleted. A VM that is already stopped is left alone. Two NIC
// deletes can run at once; a kill that loses that race still continues
// when the VM is stopped.
func (na *NICApi) killVMForNICDelete(ctx context.Context, vmId types.String) error {
	running, err := readVMPowerState(ctx, na.client, vmId)
	if err != nil {
		return fmt.Errorf("checking VM power state before killing it: %w", err)
	}
	if !running {
		tflog.Debug(ctx, fmt.Sprintf("VM %s is already stopped", vmId.ValueString()))
		return nil
	}

	vmAPI, err := NewVMApi(na.client)
	if err != nil {
		return err
	}
	if err := vmAPI.killVM(ctx, &VMResourceModel{Id: vmId}); err != nil {
		stillRunning, readErr := readVMPowerState(ctx, na.client, vmId)
		if readErr == nil && !stillRunning {
			tflog.Debug(ctx, fmt.Sprintf("VM %s is stopped after kill returned %v", vmId.ValueString(), err))
			return nil
		}
		return fmt.Errorf("killing VM %s so the NIC can be deleted: %w", vmId.ValueString(), err)
	}
	return nil
}

func (na *NICApi) removeNIC(ctx context.Context, id string) error {
	_, apiErr := na.client.Delete(ctx, vergeio.ObjectPath(NICEndpoint, id))
	if apiErr != nil {
		return errors.New("Error deleting the NIC: " + apiErr.Error())
	}
	tflog.Debug(ctx, "NIC was successfully deleted")
	return nil
}

// removeStoppedVMNIC deletes a NIC whose VM is not running.
// Power-off may not have flipped the status to down yet, so this polls
// briefly. The NIC is still deleted if it stays up: no guest is left to
// finish the unplug, and failing here blocks VM destroy.
func (na *NICApi) removeStoppedVMNIC(ctx context.Context, id, name string) error {
	status, err := waitForStatusWithin(ctx, func() (string, error) {
		var current string
		readErr := na.checkNICPowerState(ctx, id, &current)
		return current, readErr
	}, "down", nicIgnoredUnplugTimeout, deviceDetachInterval)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if status == "" {
			return fmt.Errorf("NIC %s status could not be read after the VM stopped: %w", detachLabel(name, id, "id"), err)
		}
		tflog.Debug(ctx, fmt.Sprintf("NIC %s stayed %q with the VM stopped; deleting it", detachLabel(name, id, "id"), status))
	}
	return na.removeNIC(ctx, id)
}

// nicBlockID is the NIC id stored in state. An empty id means the block has
// no id yet and pairBlocks may match it by name.
func nicBlockID(nic *nicResourceModel) string {
	if nic == nil || nic.Id.IsNull() || nic.Id.IsUnknown() {
		return ""
	}
	return nic.Id.ValueString()
}

func nicBlockName(nic *nicResourceModel) string {
	if nic == nil {
		return ""
	}
	return nic.Name.ValueString()
}

// nicNeedsUpdate reports a difference the NIC update API can PUT.
// Name is included so a rename is an update of the existing id. The MAC is
// sent only when the configuration changes it; a rename leaves it out, so
// the platform keeps the address.
// A null or unknown plan value is not a change. Optional+Computed attributes
// are unknown on an update that does not set them, and ValueString, ValueBool,
// and ValueInt32 read those as "", false, or 0. Comparing the zero values
// would PUT every NIC on any VM change.
func nicNeedsUpdate(plan, state *nicResourceModel) bool {
	return vergeio.ChangedString(plan.Name, state.Name) != nil ||
		vergeio.ChangedString(plan.Description, state.Description) != nil ||
		vergeio.ChangedString(plan.Interface, state.Interface) != nil ||
		vergeio.ChangedString(plan.Driver, state.Driver) != nil ||
		vergeio.ChangedString(plan.Model, state.Model) != nil ||
		vergeio.ChangedString(plan.Vendor, state.Vendor) != nil ||
		vergeio.ChangedInt32(plan.Port, state.Port) != nil ||
		vergeio.ChangedInt32(plan.VNET, state.VNET) != nil ||
		vergeio.ChangedString(plan.MAC, state.MAC) != nil ||
		vergeio.ChangedString(plan.Asset, state.Asset) != nil ||
		vergeio.ChangedBool(plan.Enabled, state.Enabled) != nil
}

// Update, Create, Delete the NIC in the API.
// This method is called from VM update method.
func (na *NICApi) syncNICs(ctx context.Context, planData *[]*nicResourceModel, stateData *[]*nicResourceModel, machine types.Int32, vmId types.String) error {
	plan := *planData
	state := *stateData
	updates, creates, deletes := pairBlocks(plan, state, nicBlockID, nicBlockName)

	tflog.Debug(ctx, "Syncing NICs: Starting deletion")
	for _, old := range deletes {
		if err := na.deleteNIC(ctx, old, vmId); err != nil {
			return fmt.Errorf("failed to delete nic: %v", err)
		}
	}

	tflog.Debug(ctx, "Syncing NICs: Starting updation")
	for _, pair := range updates {
		if !nicNeedsUpdate(pair.plan, pair.state) {
			continue
		}
		if err := na.updateNIC(ctx, pair.plan, pair.state); err != nil {
			return fmt.Errorf("failed to update nic: %v", err)
		}
	}

	// A NIC created while the VM is stopped is present at the next boot.
	// Update powers the VM on only after this sync. A NIC created while the
	// VM is already running stays down until hotplugnic.
	vmRunning := false
	if len(creates) > 0 {
		running, err := readVMPowerState(ctx, na.client, vmId)
		if err != nil {
			return fmt.Errorf("failed to read VM power state before adding NICs: %w", err)
		}
		vmRunning = running
	}

	tflog.Debug(ctx, "Syncing NICs: Starting insertion")
	for _, created := range creates {
		created.Machine = machine
		if err := na.createNIC(ctx, created); err != nil {
			return fmt.Errorf("failed to create nic: %v", err)
		}
		if err := na.readNIC(ctx, created); err != nil {
			return fmt.Errorf("failed to read disk during the sync: %v", err)
		}
		if vmRunning {
			if err := na.attachCreatedNIC(ctx, created, vmId); err != nil {
				return err
			}
		}
	}

	*stateData = syncedOrder(plan, updates)
	return nil
}

// to get the power status of the nic.
func (na *NICApi) checkNICPowerState(ctx context.Context, key string, powerState *string) error {

	tflog.Debug(ctx, fmt.Sprintf("Checking the power state of the NIC %v", key))

	// Call the API to get the NIC status
	apiResp, err := na.client.Get(ctx, vergeio.ObjectPath(NICEndpoint, key),
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
// Unset (null or unknown) attributes are omitted. An explicit false, 0, or ""
// is sent. Enabled stays omitted when the configuration does not set it, so
// the platform default is kept, and enabled=false is still sent when set.
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
	if value.IsNull() || value.IsUnknown() {
		return
	}
	payload[key] = value.ValueString()
}

func putKnownInt32(payload map[string]any, key string, value types.Int32) {
	if value.IsNull() || value.IsUnknown() {
		return
	}
	payload[key] = value.ValueInt32()
}

// nicUpdatePayload is the JSON body for NIC update.
// Unset attributes are omitted. An explicit false, 0, or "" is sent when it
// differs from state.
func nicUpdatePayload(planData *nicResourceModel, stateData *nicResourceModel) nicAPIResourceModel {
	return nicAPIResourceModel{
		Machine:     vergeio.ChangedInt32(planData.Machine, stateData.Machine),
		Name:        vergeio.ChangedString(planData.Name, stateData.Name),
		Description: vergeio.ChangedString(planData.Description, stateData.Description),
		Interface:   vergeio.ChangedString(planData.Interface, stateData.Interface),
		Driver:      vergeio.ChangedString(planData.Driver, stateData.Driver),
		Model:       vergeio.ChangedString(planData.Model, stateData.Model),
		Vendor:      vergeio.ChangedString(planData.Vendor, stateData.Vendor),
		Port:        vergeio.ChangedInt32(planData.Port, stateData.Port),
		Enabled:     vergeio.ChangedBool(planData.Enabled, stateData.Enabled),
		VNET:        vergeio.ChangedInt32(planData.VNET, stateData.VNET),
		MAC:         vergeio.ChangedString(planData.MAC, stateData.MAC),
		Asset:       vergeio.ChangedString(planData.Asset, stateData.Asset),
	}
}

func putKnownBool(payload map[string]any, key string, value types.Bool) {
	if value.IsNull() || value.IsUnknown() {
		return
	}
	payload[key] = value.ValueBool()
}

// attachCreatedNIC hotplugs a NIC that was just created on a running VM and
// waits until it is up. A disabled NIC is left down. When VergeOS refuses
// the action, or the NIC never comes up, the error tells the caller to
// power cycle the VM.
func (na *NICApi) attachCreatedNIC(ctx context.Context, nic *nicResourceModel, vmId types.String) error {
	if nic == nil || !deviceShouldAttach(nic.Enabled) {
		tflog.Debug(ctx, "Skipping hotplug for a disabled NIC")
		return nil
	}

	name := nic.Name.ValueString()
	iface := nic.Interface.ValueString()
	vmAPI, err := NewVMApi(na.client)
	if err != nil {
		return err
	}
	if err := vmAPI.hotplugNIC(ctx, nic.Id.ValueString(), vmId, false); err != nil {
		return fmt.Errorf("NIC %q (interface %q) was created on running VM %s, but VergeOS refused to hotplug it: %w. Power cycle the VM so the guest can see the NIC", name, iface, vmId.ValueString(), err)
	}

	status, err := waitForStatus(ctx, func() (string, error) {
		var current string
		readErr := na.checkNICPowerState(ctx, nic.Id.ValueString(), &current)
		return current, readErr
	}, "up")
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if status == "" {
			return fmt.Errorf("NIC %q was hotplugged but its status could not be read: %w", name, err)
		}
		return fmt.Errorf("NIC %q (interface %q) stayed %q after hotplug on running VM %s. Power cycle the VM so the guest can see the NIC", name, iface, status, vmId.ValueString())
	}
	return nil
}

// hotplugNIC sends hotplugnic. unplug detaches the NIC; otherwise the action
// attaches a NIC that is already assigned to the VM.
func (va *VMApi) hotplugNIC(ctx context.Context, nicId string, vmId types.String, unplug bool) error {

	tflog.Debug(ctx, fmt.Sprintf("Calling the VMActions API for the NIC %v unplug %v", nicId, unplug))

	// Create the action payload according to vm_actions schema.
	// Unplug is omitted when attaching. The API treats a missing unplug flag
	// as hotplug, which brings a down NIC up.
	params := VMActionParams{
		Device: nicId,
	}
	if unplug {
		params.Unplug = true
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
	req, err := va.client.Post(ctx, VMActionEndpoint, bytes.NewBuffer(bytedata))
	if err != nil {
		return err
	}
	if req.StatusCode != 201 {
		return fmt.Errorf("failed to hotplug NIC: status code %v", req.StatusCode)
	}

	return nil
}

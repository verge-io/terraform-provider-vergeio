// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

var _ datasource.DataSource = &VMSnapshotsDataSource{}

func NewVMSnapshotsDataSource() datasource.DataSource {
	return &VMSnapshotsDataSource{}
}

// VMSnapshotsDataSource is vergeio_vm_snapshots.
type VMSnapshotsDataSource struct {
	api *VMApi
}

// VMSnapshotListModel is one machine_snapshots row.
type VMSnapshotListModel struct {
	Id          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Created     types.Int64  `tfsdk:"created"`
	Expires     types.Int64  `tfsdk:"expires"`
	Machine     types.Int64  `tfsdk:"machine"`
	SnapMachine types.Int64  `tfsdk:"snap_machine"`
}

// VMSnapshotsDataSourceModel is vergeio_vm_snapshots.
type VMSnapshotsDataSourceModel struct {
	VMID      types.String          `tfsdk:"vm_id"`
	Snapshots []VMSnapshotListModel `tfsdk:"snapshots"`
}

func (d *VMSnapshotsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_snapshots"
}

func (d *VMSnapshotsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists machine_snapshots for one VM. vm_id is the VM $key, vergeio_vm.id, not the machine key. The list is ordered by created, then by snapshot key. This data source does not create or change a snapshot.",
		Attributes: map[string]schema.Attribute{
			"vm_id": schema.StringAttribute{
				MarkdownDescription: "VM $key, the same value as vergeio_vm.id.",
				Required:            true,
			},
			"snapshots": schema.ListNestedAttribute{
				MarkdownDescription: "Snapshots of this VM, including ones this configuration did not create.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":           schema.StringAttribute{MarkdownDescription: "machine_snapshots key.", Computed: true},
						"name":         schema.StringAttribute{MarkdownDescription: "Snapshot name.", Computed: true},
						"created":      schema.Int64Attribute{MarkdownDescription: "Creation time as a Unix timestamp.", Computed: true},
						"expires":      schema.Int64Attribute{MarkdownDescription: "Expiration time as a Unix timestamp. Null when the snapshot does not expire.", Computed: true},
						"machine":      schema.Int64Attribute{MarkdownDescription: "Parent VM machine key. machine_snapshots.machine, not the VM $key.", Computed: true},
						"snap_machine": schema.Int64Attribute{MarkdownDescription: "Machine key of the snapshot VM (is_snapshot true). vergeio_vm_clone clones that VM.", Computed: true},
					},
				},
			},
		},
	}
}

func (d *VMSnapshotsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*vergeio.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *vergeio.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	api, err := NewVMApi(client)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create VergeOS API Client", err.Error())
		return
	}
	d.api = api
}

func (d *VMSnapshotsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data VMSnapshotsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, _, err := shared.KnownPositiveID(data.VMID, "vm_id"); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("vm_id"), "Invalid VM ID", err.Error())
		return
	}
	if err := d.api.readVMSnapshots(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading VM snapshots", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (va *VMApi) readVMSnapshots(ctx context.Context, data *VMSnapshotsDataSourceModel) error {
	if va == nil || va.sdk == nil {
		return fmt.Errorf("VM client is not configured")
	}
	vmID, known, err := shared.KnownPositiveID(data.VMID, "vm_id")
	if err != nil || !known {
		if err == nil {
			err = fmt.Errorf("vm_id is required")
		}
		return err
	}
	snaps, err := va.sdk.VMSnapshots.ListByVM(ctx, vmID)
	if err != nil {
		return err
	}
	sort.Slice(snaps, func(i, j int) bool {
		if snaps[i].Created != snaps[j].Created {
			return snaps[i].Created < snaps[j].Created
		}
		return snaps[i].Key.Int() < snaps[j].Key.Int()
	})
	models := make([]VMSnapshotListModel, 0, len(snaps))
	for i := range snaps {
		models = append(models, vmSnapshotListModel(&snaps[i]))
	}
	data.Snapshots = models
	tflog.Debug(ctx, fmt.Sprintf("read %d VM snapshots for VM %d", len(models), vmID))
	return nil
}

func vmSnapshotListModel(snap *vergeos.VMSnapshot) VMSnapshotListModel {
	return VMSnapshotListModel{
		Id:          types.StringValue(strconv.Itoa(snap.Key.Int())),
		Name:        types.StringValue(snap.Name),
		Created:     types.Int64Value(snap.Created),
		Expires:     unixOrNull(snap.Expires),
		Machine:     types.Int64Value(int64(snap.Machine.Int())),
		SnapMachine: types.Int64Value(int64(snap.SnapMachine.Int())),
	}
}

func unixOrNull(v int64) types.Int64 {
	if v == 0 {
		return types.Int64Null()
	}
	return types.Int64Value(v)
}

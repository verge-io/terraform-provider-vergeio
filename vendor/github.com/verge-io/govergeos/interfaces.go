package vergeos

import (
	"context"
	"io"
	"time"
)

// This file defines interfaces for all services to enable mock testing and dependency injection.
// See ADR-012 in DECISIONS.md for design rationale.

// VMServiceInterface defines the interface for VM operations.
type VMServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VM, error)
	Get(ctx context.Context, id int) (*VM, error)
	Create(ctx context.Context, req *VMCreateRequest) (*VM, error)
	Update(ctx context.Context, id int, req *VMUpdateRequest) (*VM, error)
	Delete(ctx context.Context, id int) error
	PowerOn(ctx context.Context, id int) error
	PowerOff(ctx context.Context, id int) error
	Reset(ctx context.Context, id int) error
	GuestReboot(ctx context.Context, id int) error
	GuestShutdown(ctx context.Context, id int) error
	Clone(ctx context.Context, id int, opts *VMCloneOptions) error
	Snapshot(ctx context.Context, id int, opts *VMSnapshotOptions) error
	Migrate(ctx context.Context, id int, opts *VMMigrateOptions) error
	GetConsoleURL(ctx context.Context, id int) (string, error)
}

// VMSnapshotServiceInterface defines the interface for VM Snapshot operations.
type VMSnapshotServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VMSnapshot, error)
	ListByVM(ctx context.Context, vmID int, opts ...ListOption) ([]VMSnapshot, error)
	ListExpiring(ctx context.Context, days int, opts ...ListOption) ([]VMSnapshot, error)
	Get(ctx context.Context, id int) (*VMSnapshot, error)
	GetByName(ctx context.Context, vmID int, name string) (*VMSnapshot, error)
	Create(ctx context.Context, req *VMSnapshotCreateRequest) (*VMSnapshot, error)
	Update(ctx context.Context, id int, req *VMSnapshotUpdateRequest) (*VMSnapshot, error)
	Delete(ctx context.Context, id int) error
	Restore(ctx context.Context, id int, opts *VMSnapshotRestoreOptions) error
	SetNeverExpires(ctx context.Context, id int) (*VMSnapshot, error)
	SetExpires(ctx context.Context, id int, expires int64) (*VMSnapshot, error)
}

// VMNICServiceInterface defines the interface for VM NIC operations.
type VMNICServiceInterface interface {
	List(ctx context.Context, vmID int) ([]VMNIC, error)
	Get(ctx context.Context, nicID int) (*VMNIC, error)
	Create(ctx context.Context, vmID int, req *VMNICCreateRequest) (*VMNIC, error)
	Update(ctx context.Context, nicID int, req *VMNICUpdateRequest) (*VMNIC, error)
	Delete(ctx context.Context, nicID int) error
}

// VMDriveServiceInterface defines the interface for VM Drive operations.
type VMDriveServiceInterface interface {
	List(ctx context.Context, vmID int) ([]VMDrive, error)
	Get(ctx context.Context, driveID int) (*VMDrive, error)
	Create(ctx context.Context, vmID int, req *VMDriveCreateRequest) (*VMDrive, error)
	Update(ctx context.Context, driveID int, req *VMDriveUpdateRequest) (*VMDrive, error)
	Delete(ctx context.Context, driveID int) error
}

// VMDeviceServiceInterface defines the interface for VM Device operations.
type VMDeviceServiceInterface interface {
	List(ctx context.Context, vmID int) ([]VMDevice, error)
	Get(ctx context.Context, deviceID int) (*VMDevice, error)
	Create(ctx context.Context, vmID int, req *VMDeviceCreateRequest) (*VMDevice, error)
	Update(ctx context.Context, deviceID int, req *VMDeviceUpdateRequest) (*VMDevice, error)
	Delete(ctx context.Context, deviceID int) error
}

// NetworkServiceInterface defines the interface for Network operations.
type NetworkServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]Network, error)
	Get(ctx context.Context, id int) (*Network, error)
	Create(ctx context.Context, req *NetworkCreateRequest) (*Network, error)
	Update(ctx context.Context, id int, req *NetworkUpdateRequest) (*Network, error)
	Delete(ctx context.Context, id int) error
	PowerOn(ctx context.Context, id int) error
	PowerOff(ctx context.Context, id int) error
	Kill(ctx context.Context, id int) error
	Reset(ctx context.Context, id int, applyFirewall bool) error
	ApplyRules(ctx context.Context, id int) error
	ApplyDNS(ctx context.Context, id int) error
	// Diagnostics
	RunQuery(ctx context.Context, req *NetworkQueryRequest) (*NetworkQuery, error)
	GetQuery(ctx context.Context, id string) (*NetworkQuery, error)
	RunQueryWait(ctx context.Context, req *NetworkQueryRequest) (*NetworkQuery, error)
	Ping(ctx context.Context, networkID int, target string, count int) (*NetworkQuery, error)
	Traceroute(ctx context.Context, networkID int, target string) (*NetworkQuery, error)
	DNSLookup(ctx context.Context, networkID int, hostname string) (*NetworkQuery, error)
	GetDiagnostics(ctx context.Context, id int) (*NetworkQuery, error)
	// Statistics
	GetStatistics(ctx context.Context, id int) ([]NetworkMonitorStats, error)
	GetLatestStatistics(ctx context.Context, id int) (*NetworkMonitorStats, error)
}

// UserServiceInterface defines the interface for User operations.
type UserServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]User, error)
	Get(ctx context.Context, id int) (*User, error)
	GetByName(ctx context.Context, name string) (*User, error)
	Create(ctx context.Context, req *UserCreateRequest) (*User, error)
	Update(ctx context.Context, id int, req *UserUpdateRequest) (*User, error)
	Delete(ctx context.Context, id int) error
	Enable(ctx context.Context, id int) error
	Disable(ctx context.Context, id int) error
}

// MemberServiceInterface defines the interface for Member operations.
type MemberServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]Member, error)
	ListByGroup(ctx context.Context, groupID int) ([]Member, error)
	Get(ctx context.Context, id int) (*Member, error)
	Create(ctx context.Context, req *MemberCreateRequest) (*Member, error)
	Update(ctx context.Context, id int, req *MemberUpdateRequest) (*Member, error)
	Delete(ctx context.Context, id int) error
	Add(ctx context.Context, groupID int, member string) (*Member, error)
	Remove(ctx context.Context, groupID int, member string) error
}

// CloudInitServiceInterface defines the interface for CloudInit operations.
type CloudInitServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]CloudInitFile, error)
	Get(ctx context.Context, id int) (*CloudInitFile, error)
	GetByName(ctx context.Context, name string) (*CloudInitFile, error)
	Create(ctx context.Context, req *CloudInitFileCreateRequest) (*CloudInitFile, error)
	Update(ctx context.Context, id int, req *CloudInitFileUpdateRequest) (*CloudInitFile, error)
	Delete(ctx context.Context, id int) error
}

// NodeServiceInterface defines the interface for Node operations.
type NodeServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]Node, error)
	ListPhysical(ctx context.Context, opts ...ListOption) ([]Node, error)
	Get(ctx context.Context, id int) (*Node, error)
	GetByName(ctx context.Context, name string) (*Node, error)
	GetDashboard(ctx context.Context, id int) (*Node, error)
	EnableMaintenance(ctx context.Context, id int) error
	DisableMaintenance(ctx context.Context, id int) error
	MaintenanceReboot(ctx context.Context, id int) error
	ClearPStore(ctx context.Context, id int) error
}

// ClusterServiceInterface defines the interface for Cluster operations.
type ClusterServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]Cluster, error)
	Get(ctx context.Context, id int) (*Cluster, error)
	GetByName(ctx context.Context, name string) (*Cluster, error)
	GetStatus(ctx context.Context, id int) (*ClusterStatus, error)
	Create(ctx context.Context, req *ClusterCreateRequest) (*Cluster, error)
	Update(ctx context.Context, id int, req *ClusterUpdateRequest) (*Cluster, error)
	Delete(ctx context.Context, id int) error
}

// GroupServiceInterface defines the interface for Group operations.
type GroupServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]Group, error)
	Get(ctx context.Context, id int) (*Group, error)
	GetByName(ctx context.Context, name string) (*Group, error)
	Create(ctx context.Context, req *GroupCreateRequest) (*Group, error)
	Update(ctx context.Context, id int, req *GroupUpdateRequest) (*Group, error)
	Delete(ctx context.Context, id int) error
}

// FileServiceInterface defines the interface for File operations.
type FileServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]File, error)
	Get(ctx context.Context, id int) (*File, error)
	GetByName(ctx context.Context, name string) (*File, error)
	ListISOs(ctx context.Context, opts ...ListOption) ([]File, error)
	Create(ctx context.Context, req *FileCreateRequest) (*File, error)
	Update(ctx context.Context, id int, req *FileUpdateRequest) (*File, error)
	Delete(ctx context.Context, id int) error
	Download(ctx context.Context, id int) (io.ReadCloser, *File, error)
	DownloadToFile(ctx context.Context, id int, destPath string) (string, error)
	Upload(ctx context.Context, id int, reader io.Reader, size int64) (*File, error)
	UploadWithChunkSize(ctx context.Context, id int, reader io.Reader, size int64, chunkSize int) (*File, error)
	UploadFromFile(ctx context.Context, localPath string, req *FileCreateRequest) (*File, error)
}

// ResourceGroupServiceInterface defines the interface for ResourceGroup operations.
type ResourceGroupServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]ResourceGroup, error)
	Get(ctx context.Context, id int) (*ResourceGroup, error)
	GetByName(ctx context.Context, name string) (*ResourceGroup, error)
}

// SettingsServiceInterface defines the interface for Settings operations.
type SettingsServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]Setting, error)
	Get(ctx context.Context, id int) (*Setting, error)
	GetByKey(ctx context.Context, key string) (*Setting, error)
	GetValue(ctx context.Context, key string) (string, error)
	GetCloudName(ctx context.Context) (string, error)
}

// SystemServiceInterface defines the interface for System operations.
type SystemServiceInterface interface {
	GetInfo(ctx context.Context) (*SystemInfo, error)
	GetVersion(ctx context.Context) (string, error)
}

// SchemaServiceInterface defines the interface for Schema operations.
type SchemaServiceInterface interface {
	GetTableSchema(ctx context.Context, resource string) (*TableSchema, error)
	GetValidValues(ctx context.Context, resource, field string) (map[string]string, error)
	GetVMMachineTypes(ctx context.Context) (map[string]string, error)
	GetVMOSFamilies(ctx context.Context) (map[string]string, error)
}

// TagServiceInterface defines the interface for Tag operations.
type TagServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]Tag, error)
	Get(ctx context.Context, id int) (*Tag, error)
	GetByName(ctx context.Context, name string) (*Tag, error)
	ListByCategory(ctx context.Context, categoryID int) ([]Tag, error)
	Create(ctx context.Context, req *TagCreateRequest) (*Tag, error)
	Update(ctx context.Context, id int, req *TagUpdateRequest) (*Tag, error)
	Delete(ctx context.Context, id int) error
}

// TagCategoryServiceInterface defines the interface for TagCategory operations.
type TagCategoryServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]TagCategory, error)
	Get(ctx context.Context, id int) (*TagCategory, error)
	GetByName(ctx context.Context, name string) (*TagCategory, error)
	Create(ctx context.Context, req *TagCategoryCreateRequest) (*TagCategory, error)
	Update(ctx context.Context, id int, req *TagCategoryUpdateRequest) (*TagCategory, error)
	Delete(ctx context.Context, id int) error
}

// TagMemberServiceInterface defines the interface for TagMember operations.
type TagMemberServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]TagMember, error)
	ListByTag(ctx context.Context, tagID int) ([]TagMember, error)
	ListByMember(ctx context.Context, member string) ([]TagMember, error)
	Get(ctx context.Context, id int) (*TagMember, error)
	Create(ctx context.Context, req *TagMemberCreateRequest) (*TagMember, error)
	Update(ctx context.Context, id int, req *TagMemberUpdateRequest) (*TagMember, error)
	Delete(ctx context.Context, id int) error
	Assign(ctx context.Context, tagID int, member string) (*TagMember, error)
	Unassign(ctx context.Context, tagID int, member string) error
}

// VolumeServiceInterface defines the interface for Volume operations.
// Note: Unlike other resources, volumes use SHA1 hash strings as IDs instead of integers.
type VolumeServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]Volume, error)
	ListByService(ctx context.Context, serviceID int) ([]Volume, error)
	Get(ctx context.Context, id string) (*Volume, error)
	GetByName(ctx context.Context, serviceID int, name string) (*Volume, error)
	Create(ctx context.Context, req *VolumeCreateRequest) (*Volume, error)
	Update(ctx context.Context, id string, req *VolumeUpdateRequest) (*Volume, error)
	Delete(ctx context.Context, id string) error
	Enable(ctx context.Context, id string) error
	Disable(ctx context.Context, id string) error
	Reset(ctx context.Context, id string) error
}

// VNetRuleServiceInterface defines the interface for network firewall rule operations.
type VNetRuleServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VNetRule, error)
	ListByNetwork(ctx context.Context, vnetID int, opts ...ListOption) ([]VNetRule, error)
	Get(ctx context.Context, id int) (*VNetRule, error)
	GetByName(ctx context.Context, vnetID int, name string) (*VNetRule, error)
	Create(ctx context.Context, req *VNetRuleCreateRequest) (*VNetRule, error)
	Update(ctx context.Context, id int, req *VNetRuleUpdateRequest) (*VNetRule, error)
	Delete(ctx context.Context, id int) error
	Enable(ctx context.Context, id int, apply bool) error
	Disable(ctx context.Context, id int, apply bool) error
}

// VNetRuleAliasServiceInterface defines the interface for network rule alias operations.
type VNetRuleAliasServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VNetRuleAlias, error)
	Get(ctx context.Context, id int) (*VNetRuleAlias, error)
	GetByName(ctx context.Context, name string) (*VNetRuleAlias, error)
	Create(ctx context.Context, req *VNetRuleAliasCreateRequest) (*VNetRuleAlias, error)
	Update(ctx context.Context, id int, req *VNetRuleAliasUpdateRequest) (*VNetRuleAlias, error)
	Delete(ctx context.Context, id int) error
}

// TenantServiceInterface defines the interface for Tenant operations.
type TenantServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]Tenant, error)
	Get(ctx context.Context, id int) (*Tenant, error)
	GetByName(ctx context.Context, name string) (*Tenant, error)
	Create(ctx context.Context, req *TenantCreateRequest) (*Tenant, error)
	Update(ctx context.Context, id int, req *TenantUpdateRequest) (*Tenant, error)
	Delete(ctx context.Context, id int) error
	PowerOn(ctx context.Context, id int) error
	PowerOnWithNode(ctx context.Context, id int, preferredNode int) error
	PowerOff(ctx context.Context, id int) error
	Reset(ctx context.Context, id int) error
	Clone(ctx context.Context, id int, opts *TenantCloneOptions) error
	IsolateOn(ctx context.Context, id int) error
	IsolateOff(ctx context.Context, id int) error
}

// TenantNodeServiceInterface defines the interface for TenantNode operations.
type TenantNodeServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]TenantNode, error)
	ListByTenant(ctx context.Context, tenantID int, opts ...ListOption) ([]TenantNode, error)
	Get(ctx context.Context, id int) (*TenantNode, error)
	GetByName(ctx context.Context, tenantID int, name string) (*TenantNode, error)
	Create(ctx context.Context, req *TenantNodeCreateRequest) (*TenantNode, error)
	Update(ctx context.Context, id int, req *TenantNodeUpdateRequest) (*TenantNode, error)
	Delete(ctx context.Context, id int) error
	PowerOn(ctx context.Context, id int) error
	PowerOff(ctx context.Context, id int) error
	Reset(ctx context.Context, id int) error
	Kill(ctx context.Context, id int) error
	Migrate(ctx context.Context, id int, targetNode int) error
}

// TenantStorageServiceInterface defines the interface for TenantStorage operations.
type TenantStorageServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]TenantStorage, error)
	ListByTenant(ctx context.Context, tenantID int, opts ...ListOption) ([]TenantStorage, error)
	Get(ctx context.Context, id int) (*TenantStorage, error)
	Create(ctx context.Context, req *TenantStorageCreateRequest) (*TenantStorage, error)
	Update(ctx context.Context, id int, req *TenantStorageUpdateRequest) (*TenantStorage, error)
	Delete(ctx context.Context, id int) error
}

// SnapshotProfileServiceInterface defines the interface for SnapshotProfile operations.
type SnapshotProfileServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]SnapshotProfile, error)
	Get(ctx context.Context, id int) (*SnapshotProfile, error)
	GetByName(ctx context.Context, name string) (*SnapshotProfile, error)
	Create(ctx context.Context, req *SnapshotProfileCreateRequest) (*SnapshotProfile, error)
	Update(ctx context.Context, id int, req *SnapshotProfileUpdateRequest) (*SnapshotProfile, error)
	Delete(ctx context.Context, id int) error
}

// SnapshotProfilePeriodServiceInterface defines the interface for SnapshotProfilePeriod operations.
type SnapshotProfilePeriodServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]SnapshotProfilePeriod, error)
	ListByProfile(ctx context.Context, profileID int, opts ...ListOption) ([]SnapshotProfilePeriod, error)
	Get(ctx context.Context, id int) (*SnapshotProfilePeriod, error)
	GetByName(ctx context.Context, profileID int, name string) (*SnapshotProfilePeriod, error)
	Create(ctx context.Context, req *SnapshotProfilePeriodCreateRequest) (*SnapshotProfilePeriod, error)
	Update(ctx context.Context, id int, req *SnapshotProfilePeriodUpdateRequest) (*SnapshotProfilePeriod, error)
	Delete(ctx context.Context, id int) error
}

// AlarmServiceInterface defines the interface for Alarm operations.
type AlarmServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]Alarm, error)
	ListActive(ctx context.Context, opts ...ListOption) ([]Alarm, error)
	ListByOwner(ctx context.Context, owner string, opts ...ListOption) ([]Alarm, error)
	ListByLevel(ctx context.Context, level string, opts ...ListOption) ([]Alarm, error)
	ListByAlarmType(ctx context.Context, alarmTypeKey string, opts ...ListOption) ([]Alarm, error)
	Get(ctx context.Context, id int) (*Alarm, error)
	Update(ctx context.Context, id int, req *AlarmUpdateRequest) (*Alarm, error)
	Snooze(ctx context.Context, id int, until int64) error
	Unsnooze(ctx context.Context, id int) error
	Resolve(ctx context.Context, id int) error
	Delete(ctx context.Context, id int) error
}

// AlarmTypeServiceInterface defines the interface for AlarmType operations.
// Alarm types are read-only reference data. Note: Uses string keys, not integers.
type AlarmTypeServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]AlarmType, error)
	Get(ctx context.Context, key string) (*AlarmType, error)
	ListByLevel(ctx context.Context, level string, opts ...ListOption) ([]AlarmType, error)
}

// TaskServiceInterface defines the interface for Task operations.
type TaskServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]Task, error)
	ListRunning(ctx context.Context, opts ...ListOption) ([]Task, error)
	ListByOwner(ctx context.Context, owner string, opts ...ListOption) ([]Task, error)
	ListEnabled(ctx context.Context, opts ...ListOption) ([]Task, error)
	Get(ctx context.Context, id int) (*Task, error)
	GetByID(ctx context.Context, taskID string) (*Task, error)
	GetByName(ctx context.Context, owner, name string) (*Task, error)
	Create(ctx context.Context, req *TaskCreateRequest) (*Task, error)
	Update(ctx context.Context, id int, req *TaskUpdateRequest) (*Task, error)
	Delete(ctx context.Context, id int) error
	Execute(ctx context.Context, id int, opts *TaskExecuteOptions) error
	Enable(ctx context.Context, id int) error
	Disable(ctx context.Context, id int) error
}

// VNetAddressServiceInterface defines the interface for network IP address operations.
type VNetAddressServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VNetAddress, error)
	ListByNetwork(ctx context.Context, vnetID int, opts ...ListOption) ([]VNetAddress, error)
	ListByType(ctx context.Context, vnetID int, addrType string, opts ...ListOption) ([]VNetAddress, error)
	Get(ctx context.Context, id int) (*VNetAddress, error)
	GetByIP(ctx context.Context, vnetID int, ip string) (*VNetAddress, error)
	GetByMAC(ctx context.Context, vnetID int, mac string) (*VNetAddress, error)
	Create(ctx context.Context, req *VNetAddressCreateRequest) (*VNetAddress, error)
	Update(ctx context.Context, id int, req *VNetAddressUpdateRequest) (*VNetAddress, error)
	Delete(ctx context.Context, id int) error
}

// VNetDNSViewServiceInterface defines the interface for network DNS view operations.
type VNetDNSViewServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VNetDNSView, error)
	ListByNetwork(ctx context.Context, vnetID int, opts ...ListOption) ([]VNetDNSView, error)
	Get(ctx context.Context, id int) (*VNetDNSView, error)
	GetByName(ctx context.Context, vnetID int, name string) (*VNetDNSView, error)
	Create(ctx context.Context, req *VNetDNSViewCreateRequest) (*VNetDNSView, error)
	Update(ctx context.Context, id int, req *VNetDNSViewUpdateRequest) (*VNetDNSView, error)
	Delete(ctx context.Context, id int) error
}

// VNetDNSZoneServiceInterface defines the interface for network DNS zone operations.
type VNetDNSZoneServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VNetDNSZone, error)
	ListByView(ctx context.Context, viewID int, opts ...ListOption) ([]VNetDNSZone, error)
	Get(ctx context.Context, id int) (*VNetDNSZone, error)
	GetByDomain(ctx context.Context, viewID int, domain string) (*VNetDNSZone, error)
	Create(ctx context.Context, req *VNetDNSZoneCreateRequest) (*VNetDNSZone, error)
	Update(ctx context.Context, id int, req *VNetDNSZoneUpdateRequest) (*VNetDNSZone, error)
	Delete(ctx context.Context, id int) error
}

// VNetDNSRecordServiceInterface defines the interface for network DNS record operations.
type VNetDNSRecordServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VNetDNSRecord, error)
	ListByZone(ctx context.Context, zoneID int, opts ...ListOption) ([]VNetDNSRecord, error)
	ListByType(ctx context.Context, zoneID int, recordType string, opts ...ListOption) ([]VNetDNSRecord, error)
	Get(ctx context.Context, id int) (*VNetDNSRecord, error)
	GetByHostAndType(ctx context.Context, zoneID int, host, recordType string) (*VNetDNSRecord, error)
	Create(ctx context.Context, req *VNetDNSRecordCreateRequest) (*VNetDNSRecord, error)
	Update(ctx context.Context, id int, req *VNetDNSRecordUpdateRequest) (*VNetDNSRecord, error)
	Delete(ctx context.Context, id int) error
}

// VNetHostServiceInterface defines the interface for network host override operations.
type VNetHostServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VNetHost, error)
	ListByNetwork(ctx context.Context, vnetID int, opts ...ListOption) ([]VNetHost, error)
	Get(ctx context.Context, id int) (*VNetHost, error)
	GetByHost(ctx context.Context, vnetID int, hostname string) (*VNetHost, error)
	GetByIP(ctx context.Context, vnetID int, ip string) (*VNetHost, error)
	Create(ctx context.Context, req *VNetHostCreateRequest) (*VNetHost, error)
	Update(ctx context.Context, id int, req *VNetHostUpdateRequest) (*VNetHost, error)
	Delete(ctx context.Context, id int) error
}

// VNetWireGuardServiceInterface defines the interface for WireGuard VPN interface operations.
type VNetWireGuardServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VNetWireGuard, error)
	ListByNetwork(ctx context.Context, vnetID int, opts ...ListOption) ([]VNetWireGuard, error)
	Get(ctx context.Context, id int) (*VNetWireGuard, error)
	GetByName(ctx context.Context, vnetID int, name string) (*VNetWireGuard, error)
	Create(ctx context.Context, req *VNetWireGuardCreateRequest) (*VNetWireGuard, error)
	Update(ctx context.Context, id int, req *VNetWireGuardUpdateRequest) (*VNetWireGuard, error)
	Delete(ctx context.Context, id int) error
}

// VNetWireGuardPeerServiceInterface defines the interface for WireGuard peer operations.
type VNetWireGuardPeerServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VNetWireGuardPeer, error)
	ListByWireGuard(ctx context.Context, wireguardID int, opts ...ListOption) ([]VNetWireGuardPeer, error)
	Get(ctx context.Context, id int) (*VNetWireGuardPeer, error)
	GetByName(ctx context.Context, wireguardID int, name string) (*VNetWireGuardPeer, error)
	Create(ctx context.Context, req *VNetWireGuardPeerCreateRequest) (*VNetWireGuardPeer, error)
	Update(ctx context.Context, id int, req *VNetWireGuardPeerUpdateRequest) (*VNetWireGuardPeer, error)
	Delete(ctx context.Context, id int) error
	GetConfig(ctx context.Context, id int) (string, error)
}

// VNetWireGuardPeerStatusServiceInterface defines the interface for WireGuard peer status operations (read-only).
type VNetWireGuardPeerStatusServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VNetWireGuardPeerStatus, error)
	Get(ctx context.Context, id int) (*VNetWireGuardPeerStatus, error)
	GetByPeer(ctx context.Context, peerID int) (*VNetWireGuardPeerStatus, error)
}

// CertificateServiceInterface defines the interface for SSL/TLS certificate operations.
type CertificateServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]Certificate, error)
	Get(ctx context.Context, id int) (*Certificate, error)
	GetByDomain(ctx context.Context, domain string) (*Certificate, error)
	GetWithKeys(ctx context.Context, id int) (*Certificate, error)
	Create(ctx context.Context, req *CertificateCreateRequest) (*Certificate, error)
	Update(ctx context.Context, id int, req *CertificateUpdateRequest) (*Certificate, error)
	Delete(ctx context.Context, id int) error
	Renew(ctx context.Context, id int) (*Certificate, error)
	ListExpiring(ctx context.Context, days int, opts ...ListOption) ([]Certificate, error)
	ListValid(ctx context.Context, opts ...ListOption) ([]Certificate, error)
	ListByType(ctx context.Context, certType string, opts ...ListOption) ([]Certificate, error)
}

// VNetIPSecServiceInterface defines the interface for IPSec VPN configuration operations.
type VNetIPSecServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VNetIPSec, error)
	Get(ctx context.Context, id int) (*VNetIPSec, error)
	GetByNetwork(ctx context.Context, vnetID int) (*VNetIPSec, error)
	Create(ctx context.Context, req *VNetIPSecCreateRequest) (*VNetIPSec, error)
	Update(ctx context.Context, id int, req *VNetIPSecUpdateRequest) (*VNetIPSec, error)
	Delete(ctx context.Context, id int) error
}

// VNetIPSecPhase1ServiceInterface defines the interface for IPSec Phase 1 (IKE SA) operations.
type VNetIPSecPhase1ServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VNetIPSecPhase1, error)
	ListByIPSec(ctx context.Context, ipsecID int, opts ...ListOption) ([]VNetIPSecPhase1, error)
	Get(ctx context.Context, id int) (*VNetIPSecPhase1, error)
	GetByName(ctx context.Context, ipsecID int, name string) (*VNetIPSecPhase1, error)
	Create(ctx context.Context, req *VNetIPSecPhase1CreateRequest) (*VNetIPSecPhase1, error)
	Update(ctx context.Context, id int, req *VNetIPSecPhase1UpdateRequest) (*VNetIPSecPhase1, error)
	Delete(ctx context.Context, id int) error
}

// VNetIPSecPhase2ServiceInterface defines the interface for IPSec Phase 2 (IPsec SA) operations.
type VNetIPSecPhase2ServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VNetIPSecPhase2, error)
	ListByPhase1(ctx context.Context, phase1ID int, opts ...ListOption) ([]VNetIPSecPhase2, error)
	Get(ctx context.Context, id int) (*VNetIPSecPhase2, error)
	GetByName(ctx context.Context, phase1ID int, name string) (*VNetIPSecPhase2, error)
	Create(ctx context.Context, req *VNetIPSecPhase2CreateRequest) (*VNetIPSecPhase2, error)
	Update(ctx context.Context, id int, req *VNetIPSecPhase2UpdateRequest) (*VNetIPSecPhase2, error)
	Delete(ctx context.Context, id int) error
}

// VNetIPSecConnectionServiceInterface defines the interface for IPSec connection status operations (read-only).
type VNetIPSecConnectionServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VNetIPSecConnection, error)
	ListByNetwork(ctx context.Context, vnetID int, opts ...ListOption) ([]VNetIPSecConnection, error)
	ListByPhase1(ctx context.Context, phase1ID int, opts ...ListOption) ([]VNetIPSecConnection, error)
	Get(ctx context.Context, id int) (*VNetIPSecConnection, error)
}

// SiteServiceInterface defines the interface for Site operations.
type SiteServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]Site, error)
	Get(ctx context.Context, id int) (*Site, error)
	GetByName(ctx context.Context, name string) (*Site, error)
	GetBySiteID(ctx context.Context, siteID string) (*Site, error)
	Create(ctx context.Context, req *SiteCreateRequest) (*Site, error)
	Update(ctx context.Context, id int, req *SiteUpdateRequest) (*Site, error)
	Delete(ctx context.Context, id int) error
	Refresh(ctx context.Context, id int) error
	RefreshSettings(ctx context.Context, id int) error
	Reauthenticate(ctx context.Context, id int) error
	RunUpdates(ctx context.Context, id int) error
	ClearSyncedLogs(ctx context.Context, id int) error
}

// SiteSyncIncomingServiceInterface defines the interface for incoming sync operations.
type SiteSyncIncomingServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]SiteSyncIncoming, error)
	ListBySite(ctx context.Context, siteID int, opts ...ListOption) ([]SiteSyncIncoming, error)
	Get(ctx context.Context, id int) (*SiteSyncIncoming, error)
	GetByName(ctx context.Context, siteID int, name string) (*SiteSyncIncoming, error)
	GetBySyncID(ctx context.Context, syncID string) (*SiteSyncIncoming, error)
	Create(ctx context.Context, req *SiteSyncIncomingCreateRequest) (*SiteSyncIncoming, error)
	Update(ctx context.Context, id int, req *SiteSyncIncomingUpdateRequest) (*SiteSyncIncoming, error)
	Delete(ctx context.Context, id int) error
	Enable(ctx context.Context, id int) error
	Disable(ctx context.Context, id int) error
}

// SiteSyncOutgoingServiceInterface defines the interface for outgoing sync operations.
type SiteSyncOutgoingServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]SiteSyncOutgoing, error)
	ListBySite(ctx context.Context, siteID int, opts ...ListOption) ([]SiteSyncOutgoing, error)
	Get(ctx context.Context, id int) (*SiteSyncOutgoing, error)
	GetByName(ctx context.Context, siteID int, name string) (*SiteSyncOutgoing, error)
	Create(ctx context.Context, req *SiteSyncOutgoingCreateRequest) (*SiteSyncOutgoing, error)
	Update(ctx context.Context, id int, req *SiteSyncOutgoingUpdateRequest) (*SiteSyncOutgoing, error)
	Delete(ctx context.Context, id int) error
	Enable(ctx context.Context, id int) error
	Disable(ctx context.Context, id int) error
	Throttle(ctx context.Context, id int, throttle int) error
	DisableThrottle(ctx context.Context, id int) error
	RefreshSnapshots(ctx context.Context, id int) error
}

// SiteSyncProfilePeriodServiceInterface defines the interface for site sync profile period operations.
type SiteSyncProfilePeriodServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]SiteSyncProfilePeriod, error)
	ListByOutgoingSync(ctx context.Context, outgoingSyncID int, opts ...ListOption) ([]SiteSyncProfilePeriod, error)
	Get(ctx context.Context, id int) (*SiteSyncProfilePeriod, error)
	Create(ctx context.Context, req *SiteSyncProfilePeriodCreateRequest) (*SiteSyncProfilePeriod, error)
	Update(ctx context.Context, id int, req *SiteSyncProfilePeriodUpdateRequest) (*SiteSyncProfilePeriod, error)
	Delete(ctx context.Context, id int) error
}

// CloudSnapshotServiceInterface defines the interface for cloud snapshot (system snapshot) operations.
type CloudSnapshotServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]CloudSnapshot, error)
	ListExpiring(ctx context.Context, opts ...ListOption) ([]CloudSnapshot, error)
	ListLocal(ctx context.Context, opts ...ListOption) ([]CloudSnapshot, error)
	ListByProfile(ctx context.Context, profileID int, opts ...ListOption) ([]CloudSnapshot, error)
	Get(ctx context.Context, id int) (*CloudSnapshot, error)
	GetByName(ctx context.Context, name string) (*CloudSnapshot, error)
	Create(ctx context.Context, req *CloudSnapshotCreateRequest) (*CloudSnapshot, error)
	Update(ctx context.Context, id int, req *CloudSnapshotUpdateRequest) (*CloudSnapshot, error)
	Delete(ctx context.Context, id int) error
	Refresh(ctx context.Context, id int) error
	Clone(ctx context.Context, id int, opts *CloudSnapshotCloneOptions) error
	RequestFromProvider(ctx context.Context, id int) error
	FindTenants(ctx context.Context, id int) error
	FindVMs(ctx context.Context, id int) error
}

// CloudSnapshotVMServiceInterface defines the interface for VM listings within cloud snapshots (read-only).
type CloudSnapshotVMServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]CloudSnapshotVM, error)
	ListBySnapshot(ctx context.Context, snapshotID int, opts ...ListOption) ([]CloudSnapshotVM, error)
	Get(ctx context.Context, id int) (*CloudSnapshotVM, error)
}

// CloudSnapshotTenantServiceInterface defines the interface for tenant listings within cloud snapshots (read-only).
type CloudSnapshotTenantServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]CloudSnapshotTenant, error)
	ListBySnapshot(ctx context.Context, snapshotID int, opts ...ListOption) ([]CloudSnapshotTenant, error)
	Get(ctx context.Context, id int) (*CloudSnapshotTenant, error)
}

// VolumeCIFSShareServiceInterface defines the interface for CIFS share operations.
// Note: Like volumes, CIFS shares use SHA1 hash strings as IDs instead of integers.
type VolumeCIFSShareServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VolumeCIFSShare, error)
	ListByVolume(ctx context.Context, volumeID string, opts ...ListOption) ([]VolumeCIFSShare, error)
	Get(ctx context.Context, id string) (*VolumeCIFSShare, error)
	GetByName(ctx context.Context, volumeID, name string) (*VolumeCIFSShare, error)
	Create(ctx context.Context, req *VolumeCIFSShareCreateRequest) (*VolumeCIFSShare, error)
	Update(ctx context.Context, id string, req *VolumeCIFSShareUpdateRequest) (*VolumeCIFSShare, error)
	Delete(ctx context.Context, id string) error
}

// VolumeNFSShareServiceInterface defines the interface for NFS share operations.
// Note: Like volumes, NFS shares use SHA1 hash strings as IDs instead of integers.
type VolumeNFSShareServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VolumeNFSShare, error)
	ListByVolume(ctx context.Context, volumeID string, opts ...ListOption) ([]VolumeNFSShare, error)
	Get(ctx context.Context, id string) (*VolumeNFSShare, error)
	GetByName(ctx context.Context, volumeID, name string) (*VolumeNFSShare, error)
	Create(ctx context.Context, req *VolumeNFSShareCreateRequest) (*VolumeNFSShare, error)
	Update(ctx context.Context, id string, req *VolumeNFSShareUpdateRequest) (*VolumeNFSShare, error)
	Delete(ctx context.Context, id string) error
}

// VolumeBrowserServiceInterface defines the interface for volume file browsing operations.
// The volume browser API is asynchronous: create a job, then poll for results.
type VolumeBrowserServiceInterface interface {
	Browse(ctx context.Context, volumeID, dir string, limit int) ([]VolumeBrowserEntry, error)
	BrowseWithOptions(ctx context.Context, volumeID, dir string, limit int, offset *int, extensions string) ([]VolumeBrowserEntry, error)
	CreateJob(ctx context.Context, req *VolumeBrowserRequest) (*VolumeBrowserJob, error)
	GetJob(ctx context.Context, id string) (*VolumeBrowserJob, error)
	WaitForResult(ctx context.Context, jobID string, timeout time.Duration) ([]VolumeBrowserEntry, error)
	List(ctx context.Context, opts ...ListOption) ([]VolumeBrowserJob, error)
	ListByVolume(ctx context.Context, volumeID string, opts ...ListOption) ([]VolumeBrowserJob, error)
}

// WebhookURLServiceInterface defines the interface for webhook URL configuration operations.
type WebhookURLServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]WebhookURL, error)
	Get(ctx context.Context, id int) (*WebhookURL, error)
	GetByName(ctx context.Context, name string) (*WebhookURL, error)
	Create(ctx context.Context, req *WebhookURLCreateRequest) (*WebhookURL, error)
	Update(ctx context.Context, id int, req *WebhookURLUpdateRequest) (*WebhookURL, error)
	Delete(ctx context.Context, id int) error
	Send(ctx context.Context, id int, message string) error
}

// WebhookServiceInterface defines the interface for webhook message log operations (read-only).
type WebhookServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]Webhook, error)
	ListByWebhookURL(ctx context.Context, webhookURLID int, opts ...ListOption) ([]Webhook, error)
	ListByStatus(ctx context.Context, status string, opts ...ListOption) ([]Webhook, error)
	ListPending(ctx context.Context, opts ...ListOption) ([]Webhook, error)
	ListFailed(ctx context.Context, opts ...ListOption) ([]Webhook, error)
	Get(ctx context.Context, id int) (*Webhook, error)
	Delete(ctx context.Context, id int) error
}

// UserAPIKeyServiceInterface defines the interface for user API key operations.
type UserAPIKeyServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]UserAPIKey, error)
	ListByUser(ctx context.Context, userID int, opts ...ListOption) ([]UserAPIKey, error)
	Get(ctx context.Context, id int) (*UserAPIKey, error)
	GetByName(ctx context.Context, userID int, name string) (*UserAPIKey, error)
	Create(ctx context.Context, req *UserAPIKeyCreateRequest) (*UserAPIKey, string, error)
	Update(ctx context.Context, id int, req *UserAPIKeyUpdateRequest) (*UserAPIKey, error)
	Delete(ctx context.Context, id int) error
	ListExpired(ctx context.Context, opts ...ListOption) ([]UserAPIKey, error)
}

// NASServiceServiceInterface defines the interface for NAS service operations.
type NASServiceServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]NASService, error)
	Get(ctx context.Context, id int) (*NASService, error)
	GetByVM(ctx context.Context, vmID int) (*NASService, error)
	GetByName(ctx context.Context, name string) (*NASService, error)
	Create(ctx context.Context, req *NASServiceCreateRequest) (*NASService, error)
	Update(ctx context.Context, id int, req *NASServiceUpdateRequest) (*NASService, error)
	Delete(ctx context.Context, id int) error
}

// NASServiceUserServiceInterface defines the interface for NAS service user operations.
// Note: Like volumes, NAS service users use SHA1 hash strings as IDs instead of integers.
type NASServiceUserServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]NASServiceUser, error)
	ListByService(ctx context.Context, serviceID int) ([]NASServiceUser, error)
	Get(ctx context.Context, id string) (*NASServiceUser, error)
	GetByName(ctx context.Context, serviceID int, name string) (*NASServiceUser, error)
	Create(ctx context.Context, req *NASServiceUserCreateRequest) (*NASServiceUser, error)
	Update(ctx context.Context, id string, req *NASServiceUserUpdateRequest) (*NASServiceUser, error)
	Delete(ctx context.Context, id string) error
	Enable(ctx context.Context, id string) error
	Disable(ctx context.Context, id string) error
}

// VolumeSyncServiceInterface defines the interface for volume sync operations.
// Note: Like volumes, volume syncs use SHA1 hash strings as IDs instead of integers.
type VolumeSyncServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VolumeSync, error)
	ListByService(ctx context.Context, serviceID int) ([]VolumeSync, error)
	ListEnabled(ctx context.Context, opts ...ListOption) ([]VolumeSync, error)
	Get(ctx context.Context, id string) (*VolumeSync, error)
	GetByName(ctx context.Context, serviceID int, name string) (*VolumeSync, error)
	Create(ctx context.Context, req *VolumeSyncCreateRequest) (*VolumeSync, error)
	Update(ctx context.Context, id string, req *VolumeSyncUpdateRequest) (*VolumeSync, error)
	Delete(ctx context.Context, id string) error
	Enable(ctx context.Context, id string) error
	Disable(ctx context.Context, id string) error
	Start(ctx context.Context, id string) error
	Stop(ctx context.Context, id string) error
}

// VolumeSnapshotServiceInterface defines the interface for volume snapshot operations.
type VolumeSnapshotServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]VolumeSnapshot, error)
	ListByVolume(ctx context.Context, volumeID int, opts ...ListOption) ([]VolumeSnapshot, error)
	ListExpiring(ctx context.Context, days int, opts ...ListOption) ([]VolumeSnapshot, error)
	ListManual(ctx context.Context, opts ...ListOption) ([]VolumeSnapshot, error)
	Get(ctx context.Context, id int) (*VolumeSnapshot, error)
	GetByName(ctx context.Context, volumeID int, name string) (*VolumeSnapshot, error)
	Create(ctx context.Context, req *VolumeSnapshotCreateRequest) (*VolumeSnapshot, error)
	Update(ctx context.Context, id int, req *VolumeSnapshotUpdateRequest) (*VolumeSnapshot, error)
	Delete(ctx context.Context, id int) error
	Enable(ctx context.Context, id int) error
	Disable(ctx context.Context, id int) error
	SetNeverExpires(ctx context.Context, id int) (*VolumeSnapshot, error)
	SetExpires(ctx context.Context, id int, expires int64) (*VolumeSnapshot, error)
}

// PermissionServiceInterface defines the interface for permission operations.
type PermissionServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]Permission, error)
	ListByIdentity(ctx context.Context, identityID int, opts ...ListOption) ([]Permission, error)
	ListByTable(ctx context.Context, table string, opts ...ListOption) ([]Permission, error)
	ListByResource(ctx context.Context, table string, rowID int64, opts ...ListOption) ([]Permission, error)
	Get(ctx context.Context, id int) (*Permission, error)
	GetByIdentityAndResource(ctx context.Context, identityID int, table string, rowID int64) (*Permission, error)
	Create(ctx context.Context, req *PermissionCreateRequest) (*Permission, error)
	Update(ctx context.Context, id int, req *PermissionUpdateRequest) (*Permission, error)
	Delete(ctx context.Context, id int) error
	Grant(ctx context.Context, identityID int, table string, rowID int64, read, modify, delete bool) (*Permission, error)
	GrantReadOnly(ctx context.Context, identityID int, table string, rowID int64) (*Permission, error)
	GrantFullAccess(ctx context.Context, identityID int, table string, rowID int64) (*Permission, error)
	Revoke(ctx context.Context, identityID int, table string, rowID int64) error
}

// TenantSnapshotServiceInterface defines the interface for tenant snapshot operations.
type TenantSnapshotServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]TenantSnapshot, error)
	ListByTenant(ctx context.Context, tenantID int, opts ...ListOption) ([]TenantSnapshot, error)
	ListExpiring(ctx context.Context, days int, opts ...ListOption) ([]TenantSnapshot, error)
	Get(ctx context.Context, id int) (*TenantSnapshot, error)
	GetByName(ctx context.Context, tenantID int, name string) (*TenantSnapshot, error)
	Update(ctx context.Context, id int, req *TenantSnapshotUpdateRequest) (*TenantSnapshot, error)
	Delete(ctx context.Context, id int) error
	Refresh(ctx context.Context, tenantID int) error
	SetNeverExpires(ctx context.Context, id int) (*TenantSnapshot, error)
	SetExpires(ctx context.Context, id int, expires int64) (*TenantSnapshot, error)
}

// LogServiceInterface defines the interface for system log operations (read-only).
type LogServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]Log, error)
	ListByLevel(ctx context.Context, level string, opts ...ListOption) ([]Log, error)
	ListByObjectType(ctx context.Context, objectType string, opts ...ListOption) ([]Log, error)
	ListErrors(ctx context.Context, opts ...ListOption) ([]Log, error)
	ListAudit(ctx context.Context, opts ...ListOption) ([]Log, error)
	ListWarnings(ctx context.Context, opts ...ListOption) ([]Log, error)
	ListByUser(ctx context.Context, username string, opts ...ListOption) ([]Log, error)
	ListSince(ctx context.Context, timestampMicros int64, opts ...ListOption) ([]Log, error)
	Get(ctx context.Context, id int) (*Log, error)
	GetRecent(ctx context.Context, count int) ([]Log, error)
	GetRecentErrors(ctx context.Context, count int) ([]Log, error)
	Search(ctx context.Context, pattern string, opts ...ListOption) ([]Log, error)
}

// TenantLayer2NetworkServiceInterface defines the interface for tenant layer2 network operations.
type TenantLayer2NetworkServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]TenantLayer2Network, error)
	ListByTenant(ctx context.Context, tenantID int, opts ...ListOption) ([]TenantLayer2Network, error)
	ListByNetwork(ctx context.Context, vnetID int, opts ...ListOption) ([]TenantLayer2Network, error)
	Get(ctx context.Context, id int) (*TenantLayer2Network, error)
	GetByTenantAndNetwork(ctx context.Context, tenantID, vnetID int) (*TenantLayer2Network, error)
	Create(ctx context.Context, req *TenantLayer2NetworkCreateRequest) (*TenantLayer2Network, error)
	Update(ctx context.Context, id int, req *TenantLayer2NetworkUpdateRequest) (*TenantLayer2Network, error)
	Delete(ctx context.Context, id int) error
	Enable(ctx context.Context, id int) (*TenantLayer2Network, error)
	Disable(ctx context.Context, id int) (*TenantLayer2Network, error)
	Assign(ctx context.Context, tenantID, vnetID int) (*TenantLayer2Network, error)
	Unassign(ctx context.Context, tenantID, vnetID int) error
}

// StorageTierServiceInterface defines the interface for storage tier operations (read-only).
// Storage tiers provide system-wide VSAN storage capacity and usage information.
type StorageTierServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]StorageTier, error)
	Get(ctx context.Context, tier int) (*StorageTier, error)
}

// ClusterTierServiceInterface defines the interface for cluster tier operations (read-only).
// Cluster tiers provide cluster-specific VSAN tier status and statistics.
type ClusterTierServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]ClusterTier, error)
	ListByCluster(ctx context.Context, clusterID int, opts ...ListOption) ([]ClusterTier, error)
	Get(ctx context.Context, id int) (*ClusterTier, error)
	GetByClusterAndTier(ctx context.Context, clusterID int, tierNum int) (*ClusterTier, error)
}

// MachineDrivePhysServiceInterface defines the interface for machine drive physical status operations (read-only).
// This provides hardware-level drive information including temperature, wear level, and VSAN metrics.
type MachineDrivePhysServiceInterface interface {
	List(ctx context.Context, opts ...ListOption) ([]MachineDrivePhys, error)
	ListByVSANTier(ctx context.Context, tier int, opts ...ListOption) ([]MachineDrivePhys, error)
	ListSpares(ctx context.Context, opts ...ListOption) ([]MachineDrivePhys, error)
	ListWithWarnings(ctx context.Context, opts ...ListOption) ([]MachineDrivePhys, error)
	Get(ctx context.Context, id int) (*MachineDrivePhys, error)
	GetByParentDrive(ctx context.Context, driveID int) (*MachineDrivePhys, error)
}

// ClusterStatsHistoryServiceInterface defines the interface for cluster statistics history operations (read-only).
// This provides historical metrics for cluster monitoring.
type ClusterStatsHistoryServiceInterface interface {
	ListShort(ctx context.Context, opts ...ListOption) ([]ClusterStatsHistory, error)
	ListShortByCluster(ctx context.Context, clusterID int, opts ...ListOption) ([]ClusterStatsHistory, error)
	ListLong(ctx context.Context, opts ...ListOption) ([]ClusterStatsHistory, error)
	ListLongByCluster(ctx context.Context, clusterID int, opts ...ListOption) ([]ClusterStatsHistory, error)
	GetLatestShort(ctx context.Context, clusterID int) (*ClusterStatsHistory, error)
	GetShort(ctx context.Context, id int) (*ClusterStatsHistory, error)
	GetLong(ctx context.Context, id int) (*ClusterStatsHistory, error)
}

// Compile-time verification that concrete types satisfy their interfaces.
var (
	_ VMServiceInterface                      = (*VMService)(nil)
	_ VMSnapshotServiceInterface              = (*VMSnapshotService)(nil)
	_ VMNICServiceInterface                   = (*VMNICService)(nil)
	_ VMDriveServiceInterface                 = (*VMDriveService)(nil)
	_ VMDeviceServiceInterface                = (*VMDeviceService)(nil)
	_ NetworkServiceInterface                 = (*NetworkService)(nil)
	_ UserServiceInterface                    = (*UserService)(nil)
	_ MemberServiceInterface                  = (*MemberService)(nil)
	_ CloudInitServiceInterface               = (*CloudInitService)(nil)
	_ NodeServiceInterface                    = (*NodeService)(nil)
	_ ClusterServiceInterface                 = (*ClusterService)(nil)
	_ GroupServiceInterface                   = (*GroupService)(nil)
	_ FileServiceInterface                    = (*FileService)(nil)
	_ ResourceGroupServiceInterface           = (*ResourceGroupService)(nil)
	_ SettingsServiceInterface                = (*SettingsService)(nil)
	_ SystemServiceInterface                  = (*SystemService)(nil)
	_ SchemaServiceInterface                  = (*SchemaService)(nil)
	_ TagServiceInterface                     = (*TagService)(nil)
	_ TagCategoryServiceInterface             = (*TagCategoryService)(nil)
	_ TagMemberServiceInterface               = (*TagMemberService)(nil)
	_ VolumeServiceInterface                  = (*VolumeService)(nil)
	_ VNetRuleServiceInterface                = (*VNetRuleService)(nil)
	_ VNetRuleAliasServiceInterface           = (*VNetRuleAliasService)(nil)
	_ TenantServiceInterface                  = (*TenantService)(nil)
	_ TenantNodeServiceInterface              = (*TenantNodeService)(nil)
	_ TenantStorageServiceInterface           = (*TenantStorageService)(nil)
	_ SnapshotProfileServiceInterface         = (*SnapshotProfileService)(nil)
	_ SnapshotProfilePeriodServiceInterface   = (*SnapshotProfilePeriodService)(nil)
	_ AlarmServiceInterface                   = (*AlarmService)(nil)
	_ AlarmTypeServiceInterface               = (*AlarmTypeService)(nil)
	_ TaskServiceInterface                    = (*TaskService)(nil)
	_ VNetAddressServiceInterface             = (*VNetAddressService)(nil)
	_ VNetDNSViewServiceInterface             = (*VNetDNSViewService)(nil)
	_ VNetDNSZoneServiceInterface             = (*VNetDNSZoneService)(nil)
	_ VNetDNSRecordServiceInterface           = (*VNetDNSRecordService)(nil)
	_ VNetHostServiceInterface                = (*VNetHostService)(nil)
	_ VNetWireGuardServiceInterface           = (*VNetWireGuardService)(nil)
	_ VNetWireGuardPeerServiceInterface       = (*VNetWireGuardPeerService)(nil)
	_ VNetWireGuardPeerStatusServiceInterface = (*VNetWireGuardPeerStatusService)(nil)
	_ CertificateServiceInterface             = (*CertificateService)(nil)
	_ VNetIPSecServiceInterface               = (*VNetIPSecService)(nil)
	_ VNetIPSecPhase1ServiceInterface         = (*VNetIPSecPhase1Service)(nil)
	_ VNetIPSecPhase2ServiceInterface         = (*VNetIPSecPhase2Service)(nil)
	_ VNetIPSecConnectionServiceInterface     = (*VNetIPSecConnectionService)(nil)
	_ SiteServiceInterface                    = (*SiteService)(nil)
	_ SiteSyncIncomingServiceInterface        = (*SiteSyncIncomingService)(nil)
	_ SiteSyncOutgoingServiceInterface        = (*SiteSyncOutgoingService)(nil)
	_ SiteSyncProfilePeriodServiceInterface   = (*SiteSyncProfilePeriodService)(nil)
	_ CloudSnapshotServiceInterface           = (*CloudSnapshotService)(nil)
	_ CloudSnapshotVMServiceInterface         = (*CloudSnapshotVMService)(nil)
	_ CloudSnapshotTenantServiceInterface     = (*CloudSnapshotTenantService)(nil)
	_ VolumeCIFSShareServiceInterface         = (*VolumeCIFSShareService)(nil)
	_ VolumeNFSShareServiceInterface          = (*VolumeNFSShareService)(nil)
	_ VolumeBrowserServiceInterface           = (*VolumeBrowserService)(nil)
	_ WebhookURLServiceInterface              = (*WebhookURLService)(nil)
	_ WebhookServiceInterface                 = (*WebhookService)(nil)
	_ UserAPIKeyServiceInterface              = (*UserAPIKeyService)(nil)
	_ NASServiceServiceInterface              = (*NASServiceService)(nil)
	_ NASServiceUserServiceInterface          = (*NASServiceUserService)(nil)
	_ VolumeSyncServiceInterface              = (*VolumeSyncService)(nil)
	_ VolumeSnapshotServiceInterface          = (*VolumeSnapshotService)(nil)
	_ PermissionServiceInterface              = (*PermissionService)(nil)
	_ TenantSnapshotServiceInterface          = (*TenantSnapshotService)(nil)
	_ LogServiceInterface                     = (*LogService)(nil)
	_ TenantLayer2NetworkServiceInterface     = (*TenantLayer2NetworkService)(nil)
	_ StorageTierServiceInterface             = (*StorageTierService)(nil)
	_ ClusterTierServiceInterface             = (*ClusterTierService)(nil)
	_ MachineDrivePhysServiceInterface        = (*MachineDrivePhysService)(nil)
	_ ClusterStatsHistoryServiceInterface     = (*ClusterStatsHistoryService)(nil)
)

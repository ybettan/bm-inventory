package host

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/filanov/bm-inventory/internal/common"
	"github.com/filanov/bm-inventory/internal/network"

	"github.com/alecthomas/units"
	"github.com/filanov/bm-inventory/internal/hardware"

	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"

	"github.com/filanov/bm-inventory/models"
	logutil "github.com/filanov/bm-inventory/pkg/log"
	"github.com/filanov/stateswitch"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/thoas/go-funk"
)

type transitionHandler struct {
	db             *gorm.DB
	log            logrus.FieldLogger
	hwValidatorCfg hardware.ValidatorCfg
}

////////////////////////////////////////////////////////////////////////////
// RegisterHost
////////////////////////////////////////////////////////////////////////////

type TransitionArgsRegisterHost struct {
	ctx                   context.Context
	discoveryAgentVersion string
}

func (th *transitionHandler) PostRegisterHost(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) error {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return errors.New("PostRegisterHost incompatible type of StateSwitch")
	}
	params, ok := args.(*TransitionArgsRegisterHost)
	if !ok {
		return errors.New("PostRegisterHost invalid argument")
	}

	host := models.Host{}
	log := logutil.FromContext(params.ctx, th.log)

	// If host already exists
	if err := th.db.First(&host, "id = ? and cluster_id = ?", sHost.host.ID, sHost.host.ClusterID).Error; err == nil {
		currentState := swag.StringValue(host.Status)
		host.Status = sHost.host.Status

		// The reason for the double register is unknown (HW might have changed) -
		// so we reset the hw info and start the discovery process again.
		return updateHostStateWithParams(log, currentState, statusInfoDiscovering, &host, th.db,
			"hardware_info", "", "discovery_agent_version", params.discoveryAgentVersion)
	}

	sHost.host.StatusUpdatedAt = strfmt.DateTime(time.Now())
	sHost.host.StatusInfo = swag.String(statusInfoDiscovering)
	log.Infof("Register new host %s cluster %s", sHost.host.ID.String(), sHost.host.ClusterID)
	return th.db.Create(sHost.host).Error
}

func (th *transitionHandler) PostRegisterDuringInstallation(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) error {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return errors.New("RegisterNewHost incompatible type of StateSwitch")
	}
	params, ok := args.(*TransitionArgsRegisterHost)
	if !ok {
		return errors.New("PostRegisterDuringInstallation invalid argument")
	}
	return updateHostStateWithParams(logutil.FromContext(params.ctx, th.log), sHost.srcState,
		"The host unexpectedly restarted during the installation.", sHost.host, th.db)
}

func (th *transitionHandler) IsHostInReboot(sw stateswitch.StateSwitch, _ stateswitch.TransitionArgs) (bool, error) {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return false, errors.New("IsInReboot incompatible type of StateSwitch")
	}

	return sHost.host.Progress.CurrentStage == models.HostStageRebooting, nil
}

func (th *transitionHandler) PostRegisterDuringReboot(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) error {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return errors.New("RegisterNewHost incompatible type of StateSwitch")
	}
	params, ok := args.(*TransitionArgsRegisterHost)
	if !ok {
		return errors.New("PostRegisterDuringReboot invalid argument")
	}
	return updateHostStateWithParams(logutil.FromContext(params.ctx, th.log), sHost.srcState,
		"Expected the host to boot from disk, but it booted the installation image. Please reboot and fix boot order to boot from disk.",
		sHost.host, th.db)
}

////////////////////////////////////////////////////////////////////////////
// Installation failure
////////////////////////////////////////////////////////////////////////////

type TransitionArgsHostInstallationFailed struct {
	ctx    context.Context
	reason string
}

func (th *transitionHandler) PostHostInstallationFailed(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) error {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return errors.New("HostInstallationFailed incompatible type of StateSwitch")
	}
	params, ok := args.(*TransitionArgsHostInstallationFailed)
	if !ok {
		return errors.New("HostInstallationFailed invalid argument")
	}
	return updateHostStateWithParams(logutil.FromContext(params.ctx, th.log), sHost.srcState,
		params.reason, sHost.host, th.db)
}

////////////////////////////////////////////////////////////////////////////
// Cancel Installation
////////////////////////////////////////////////////////////////////////////

type TransitionArgsCancelInstallation struct {
	ctx    context.Context
	reason string
	db     *gorm.DB
}

func (th *transitionHandler) PostCancelInstallation(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) error {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return errors.New("PostCancelInstallation incompatible type of StateSwitch")
	}
	params, ok := args.(*TransitionArgsCancelInstallation)
	if !ok {
		return errors.New("PostCancelInstallation invalid argument")
	}
	if sHost.srcState == HostStatusError {
		return nil
	}
	return updateHostStateWithParams(logutil.FromContext(params.ctx, th.log), sHost.srcState,
		params.reason, sHost.host, params.db)
}

////////////////////////////////////////////////////////////////////////////
// Reset Host
////////////////////////////////////////////////////////////////////////////

type TransitionArgsResetHost struct {
	ctx    context.Context
	reason string
	db     *gorm.DB
}

func (th *transitionHandler) PostResetHost(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) error {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return errors.New("PostResetHost incompatible type of StateSwitch")
	}
	params, ok := args.(*TransitionArgsResetHost)
	if !ok {
		return errors.New("PostResetHost invalid argument")
	}
	return updateHostStateWithParams(logutil.FromContext(params.ctx, th.log), sHost.srcState,
		params.reason, sHost.host, params.db)
}

////////////////////////////////////////////////////////////////////////////
// Install host
////////////////////////////////////////////////////////////////////////////

type TransitionArgsInstallHost struct {
	ctx context.Context
	db  *gorm.DB
}

func (th *transitionHandler) IsValidRoleForInstallation(sw stateswitch.StateSwitch, _ stateswitch.TransitionArgs) (bool, error) {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return false, errors.New("IsValidRoleForInstallation incompatible type of StateSwitch")
	}
	validRoles := []string{string(models.HostRoleMaster), string(models.HostRoleWorker)}
	if !funk.ContainsString(validRoles, string(sHost.host.Role)) {
		return false, common.NewApiError(http.StatusConflict,
			errors.Errorf("Can't install host %s due to invalid host role: %s, should be one of %s",
				sHost.host.ID.String(), sHost.host.Role, validRoles))
	}
	return true, nil
}

func (th *transitionHandler) PostInstallHost(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) error {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return errors.New("PostInstallHost incompatible type of StateSwitch")
	}
	params, ok := args.(*TransitionArgsInstallHost)
	if !ok {
		return errors.New("PostInstallHost invalid argument")
	}
	return updateHostStateWithParams(logutil.FromContext(params.ctx, th.log), sHost.srcState, statusInfoInstalling,
		sHost.host, params.db)
}

////////////////////////////////////////////////////////////////////////////
// Disable host
////////////////////////////////////////////////////////////////////////////

type TransitionArgsDisableHost struct {
	ctx context.Context
}

func (th *transitionHandler) PostDisableHost(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) error {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return errors.New("PostDisableHost incompatible type of StateSwitch")
	}
	params, ok := args.(*TransitionArgsDisableHost)
	if !ok {
		return errors.New("PostDisableHost invalid argument")
	}
	return updateHostStateWithParams(logutil.FromContext(params.ctx, th.log), sHost.srcState, statusInfoDisabled,
		sHost.host, th.db)
}

////////////////////////////////////////////////////////////////////////////
// Enable host
////////////////////////////////////////////////////////////////////////////

type TransitionArgsEnableHost struct {
	ctx context.Context
}

func (th *transitionHandler) PostEnableHost(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) error {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return errors.New("PostEnableHost incompatible type of StateSwitch")
	}
	params, ok := args.(*TransitionArgsEnableHost)
	if !ok {
		return errors.New("PostEnableHost invalid argument")
	}
	return updateHostStateWithParams(logutil.FromContext(params.ctx, th.log), sHost.srcState, statusInfoDiscovering,
		sHost.host, th.db, "hardware_info", "")
}

////////////////////////////////////////////////////////////////////////////
// Refresh Host
////////////////////////////////////////////////////////////////////////////

type TransitionArgsRefreshHost struct {
	ctx context.Context
	db  *gorm.DB
}

// Helper functions
func (th *transitionHandler) getDb(params *TransitionArgsRefreshHost) *gorm.DB {
	if params != nil && params.db != nil {
		return params.db
	}
	return th.db
}

func dbErrorCode(err error) int32 {
	if gorm.IsRecordNotFoundError(err) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

func (th *transitionHandler) getCluster(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (*common.Cluster, error) {
	sh, ok := sw.(*stateHost)
	if !ok {
		return nil, errors.New("getCluster incompatible type of StateSwitch")
	}
	params, ok := args.(*TransitionArgsRefreshHost)
	if !ok {
		return nil, errors.New("getCluster invalid argument")
	}
	if sh.cluster == nil {
		var cluster common.Cluster
		err := th.getDb(params).Preload("Hosts", "status <> ?", HostStatusDisabled).Take(&cluster, "id = ?", sh.host.ClusterID.String()).Error
		if err != nil {
			return nil, common.NewApiError(dbErrorCode(err), errors.Wrapf(err, "Getting cluster %s", sh.host.ClusterID.String()))
		}
		sh.cluster = &cluster
	}
	return sh.cluster, nil
}

func (th *transitionHandler) getInventory(sw stateswitch.StateSwitch) (*models.Inventory, error) {
	sh, ok := sw.(*stateHost)
	if !ok {
		return nil, errors.New("getInventory incompatible type of StateSwitch")
	}
	if sh.inventory == nil {
		var inventory models.Inventory
		if err := json.Unmarshal([]byte(sh.host.Inventory), &inventory); err != nil {
			return nil, errors.Wrapf(err, "Unmarshal inventory")
		}
		sh.inventory = &inventory
	}
	return sh.inventory, nil
}

func gibToBytes(gib int64) int64 {
	return gib * int64(units.GiB)
}

// Conditions
func (th *transitionHandler) IsConnected(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (bool, error) {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return false, errors.New("IsConnected incompatible type of StateSwitch")
	}
	return sHost.host.CheckedInAt.String() == "" || time.Since(time.Time(sHost.host.CheckedInAt)) <= 3*time.Minute, nil
}

func (th *transitionHandler) HasInventory(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (bool, error) {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return false, errors.New("HasInventory incompatible type of StateSwitch")
	}
	if sHost.host.Inventory == "" {
		return false, nil
	}
	inventory, err := th.getInventory(sw)
	if err != nil {
		return false, err
	}
	if inventory.CPU == nil || inventory.Memory == nil {
		return false, errors.New("Illegal inventory.  Missing CPU or Memory")
	}
	return true, nil
}

func (th *transitionHandler) HasMinMemory(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (bool, error) {
	inventory, err := th.getInventory(sw)
	if err != nil {
		return false, err
	}
	return inventory.Memory.PhysicalBytes >= gibToBytes(th.hwValidatorCfg.MinRamGib), nil
}

func getCurrentHostName(host *models.Host, inventory *models.Inventory) string {
	if host.RequestedHostname != "" {
		return host.RequestedHostname
	}
	return inventory.Hostname
}

func (th *transitionHandler) IsHostnameUnique(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (bool, error) {
	inventory, err := th.getInventory(sw)
	if err != nil {
		return false, err
	}
	cluster, err := th.getCluster(sw, args)
	if err != nil {
		return false, err
	}
	sHost, ok := sw.(*stateHost)
	if !ok {
		return false, errors.New("IsHostnameUnique incompatible type of StateSwitch")
	}
	currentHostname := getCurrentHostName(sHost.host, inventory)
	for _, h := range cluster.Hosts {
		if h.ID.String() != sHost.host.ID.String() && h.Inventory != "" {
			var otherInventory models.Inventory
			if err := json.Unmarshal([]byte(h.Inventory), &otherInventory); err != nil {
				// It is not our hostname
				continue
			}
			if currentHostname == getCurrentHostName(h, &otherInventory) {
				return false, nil
			}
		}
	}
	return true, nil
}

func (th *transitionHandler) HasMemoryForRole(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (bool, error) {
	inventory, err := th.getInventory(sw)
	if err != nil {
		return false, err
	}
	sHost, ok := sw.(*stateHost)
	if !ok {
		return false, errors.New("HasMemoryForRole incompatible type of StateSwitch")
	}
	switch sHost.host.Role {
	case "master":
		return inventory.Memory.PhysicalBytes >= gibToBytes(th.hwValidatorCfg.MinRamGibMaster), nil
	case "worker":
		return inventory.Memory.PhysicalBytes >= gibToBytes(th.hwValidatorCfg.MinRamGibWorker), nil
	default:
		return false, errors.Errorf("Unexpected role %s", sHost.host.Role)
	}
}

func (th *transitionHandler) HasMinValidDisks(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (bool, error) {
	inventory, err := th.getInventory(sw)
	if err != nil {
		return false, err
	}
	disks := hardware.ListValidDisks(inventory, gibToBytes(th.hwValidatorCfg.MinDiskSizeGib))
	return len(disks) > 0, nil
}

func (th *transitionHandler) HasValidDisksForRole(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (bool, error) {
	// Currently they are the same
	return th.HasMinValidDisks(sw, args)
}

func (th *transitionHandler) HasMinCpuCores(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (bool, error) {
	inventory, err := th.getInventory(sw)
	if err != nil {
		return false, err
	}
	return inventory.CPU.Count >= th.hwValidatorCfg.MinCPUCores, nil
}

func (th *transitionHandler) HasCpuCoresForRole(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (bool, error) {
	inventory, err := th.getInventory(sw)
	if err != nil {
		return false, err
	}
	sHost, ok := sw.(*stateHost)
	if !ok {
		return false, errors.New("HasMemoryForRole incompatible type of StateSwitch")
	}
	switch sHost.host.Role {
	case "master":
		return inventory.CPU.Count >= th.hwValidatorCfg.MinCPUCoresMaster, nil
	case "worker":
		return inventory.CPU.Count >= th.hwValidatorCfg.MinCPUCoresWorker, nil
	default:
		return false, errors.Errorf("Unexpected role %s", sHost.host.Role)
	}
}

func (th *transitionHandler) IsMachineCidrDefined(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (bool, error) {
	cluster, err := th.getCluster(sw, args)
	if err != nil {
		return false, err
	}
	return cluster.MachineNetworkCidr != "", nil
}

func (th *transitionHandler) IsRoleDefined(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (bool, error) {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return false, errors.New("IsRoleDefined incompatible type of StateSwitch")
	}
	switch sHost.host.Role {
	case "master", "worker":
		return true, nil
	case "":
		return false, nil
	default:
		return false, errors.Errorf("Unexpected role %s", sHost.host.Role)
	}
}

func (th *transitionHandler) BelongsToMachineCIDR(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (bool, error) {
	cluster, err := th.getCluster(sw, args)
	if err != nil {
		return false, err
	}
	sHost, ok := sw.(*stateHost)
	if !ok {
		return false, errors.New("BelongsToMachineCIDR incompatible type of StateSwitch")
	}
	return network.IsHostInMachineNetCidr(th.log, cluster, sHost.host), nil
}

// Attribute getters.  Used for status-info string formatting
func (th *transitionHandler) HostId(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (interface{}, error) {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return nil, errors.New("HostId incompatible type of StateSwitch")
	}
	return sHost.host.ID.String(), nil
}

func (th *transitionHandler) ClusterId(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (interface{}, error) {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return nil, errors.New("ClusterId incompatible type of StateSwitch")
	}
	return sHost.host.ClusterID.String(), nil
}

func (th *transitionHandler) Role(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (interface{}, error) {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return nil, errors.New("Role incompatible type of StateSwitch")
	}
	return sHost.host.Role, nil
}

func (th *transitionHandler) PhysicalMemory(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (interface{}, error) {
	inventory, err := th.getInventory(sw)
	if err != nil {
		return false, err
	}
	return units.Base2Bytes(inventory.Memory.PhysicalBytes), nil
}

func (th *transitionHandler) ExpectedPhysicalMemory(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (interface{}, error) {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return nil, errors.New("ExpectedPhysicalMemory incompatible type of StateSwitch")
	}
	switch sHost.host.Role {
	case "master":
		return units.Base2Bytes(gibToBytes(th.hwValidatorCfg.MinRamGibMaster)), nil
	case "worker":
		return units.Base2Bytes(gibToBytes(th.hwValidatorCfg.MinRamGibWorker)), nil
	case "":
		return units.Base2Bytes(gibToBytes(th.hwValidatorCfg.MinRamGib)), nil

	}
	return nil, errors.Errorf("Unexpected role %s", sHost.host.Role)
}

func (th *transitionHandler) ExpectedMinDiskSize(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (interface{}, error) {
	return gibToBytes(th.hwValidatorCfg.MinDiskSizeGib), nil
}

func (th *transitionHandler) CoreCount(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (interface{}, error) {
	inventory, err := th.getInventory(sw)
	if err != nil {
		return false, err
	}
	return inventory.CPU.Count, nil
}

func (th *transitionHandler) MachineNetworkCidr(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (interface{}, error) {
	cluster, err := th.getCluster(sw, args)
	if err != nil {
		return false, err
	}
	return cluster.MachineNetworkCidr, nil
}

func (th *transitionHandler) Hostname(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (interface{}, error) {
	inventory, err := th.getInventory(sw)
	if err != nil {
		return false, err
	}
	sHost, ok := sw.(*stateHost)
	if !ok {
		return nil, errors.New("Hostname incompatible type of StateSwitch")
	}
	return getCurrentHostName(sHost.host, inventory), nil
}

func (th *transitionHandler) ExpectedCoreCount(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (interface{}, error) {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return nil, errors.New("ExpectedPhysicalMemory incompatible type of StateSwitch")
	}
	switch sHost.host.Role {
	case "master":
		return th.hwValidatorCfg.MinCPUCoresMaster, nil
	case "worker":
		return th.hwValidatorCfg.MinCPUCoresWorker, nil
	case "":
		return th.hwValidatorCfg.MinCPUCores, nil

	}
	return nil, errors.Errorf("Unexpected role %s", sHost.host.Role)
}

// Post transition callbacks

// Return a post transition function with a constant reason
func (th *transitionHandler) PostRefreshHost(reason string) stateswitch.PostTransition {
	ret := func(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) error {
		sHost, ok := sw.(*stateHost)
		if !ok {
			return errors.New("PostResetHost incompatible type of StateSwitch")
		}
		params, ok := args.(*TransitionArgsRefreshHost)
		if !ok {
			return errors.New("PostRefreshHost invalid argument")
		}
		return updateHostStateWithParams(logutil.FromContext(params.ctx, th.log), sHost.srcState,
			reason, sHost.host, params.db)
	}
	return ret
}

// Callback to save status and the formatted validation outputs as status-info to db
func (th *transitionHandler) PostValidationFailure(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs, failures map[string][]string) error {
	sHost, ok := sw.(*stateHost)
	if !ok {
		return errors.New("PostValidationFailure incompatible type of StateSwitch")
	}
	params, ok := args.(*TransitionArgsRefreshHost)
	if !ok {
		return errors.New("PostValidationFailure invalid argument")
	}
	b, err := json.Marshal(&failures)
	if err != nil {
		return errors.Wrapf(err, "While unmarshaling failures")
	}
	return updateHostStateWithParams(logutil.FromContext(params.ctx, th.log), sHost.srcState,
		string(b), sHost.host, params.db)
}

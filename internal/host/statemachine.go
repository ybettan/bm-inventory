package host

import (
	"github.com/filanov/bm-inventory/internal/statemachine"
	"github.com/filanov/stateswitch"
)

const (
	TransitionTypeRegisterHost           = "RegisterHost"
	TransitionTypeHostInstallationFailed = "HostInstallationFailed"
	TransitionTypeCancelInstallation     = "CancelInstallation"
	TransitionTypeResetHost              = "ResetHost"
	TransitionTypeInstallHost            = "InstallHost"
	TransitionTypeDisableHost            = "DisableHost"
	TransitionTypeEnableHost             = "EnableHost"
	TransitionTypeRefresh                = "RefreshHost"
)

func NewHostStateMachine(th *transitionHandler) stateswitch.StateMachine {
	sm := stateswitch.NewStateMachine()

	// Register host
	sm.AddTransition(stateswitch.TransitionRule{
		TransitionType: TransitionTypeRegisterHost,
		SourceStates: []stateswitch.State{
			"",
			HostStatusDiscovering,
			HostStatusKnown,
			HostStatusDisconnected,
			HostStatusInsufficient,
			HostStatusResetting,
		},
		DestinationState: HostStatusDiscovering,
		PostTransition:   th.PostRegisterHost,
	})

	// Register host after reboot
	sm.AddTransition(stateswitch.TransitionRule{
		TransitionType:   TransitionTypeRegisterHost,
		Condition:        th.IsHostInReboot,
		SourceStates:     []stateswitch.State{HostStatusInstallingInProgress},
		DestinationState: HostStatusInstallingPendingUserAction,
		PostTransition:   th.PostRegisterDuringReboot,
	})

	// Register host during installation
	sm.AddTransition(stateswitch.TransitionRule{
		TransitionType:   TransitionTypeRegisterHost,
		SourceStates:     []stateswitch.State{HostStatusInstalling, HostStatusInstallingInProgress},
		DestinationState: HostStatusError,
		PostTransition:   th.PostRegisterDuringInstallation,
	})

	// Installation failure
	sm.AddTransition(stateswitch.TransitionRule{
		TransitionType:   TransitionTypeHostInstallationFailed,
		SourceStates:     []stateswitch.State{HostStatusInstalling, HostStatusInstallingInProgress},
		DestinationState: HostStatusError,
		PostTransition:   th.PostHostInstallationFailed,
	})

	// Cancel installation
	sm.AddTransition(stateswitch.TransitionRule{
		TransitionType:   TransitionTypeCancelInstallation,
		SourceStates:     []stateswitch.State{HostStatusInstalling, HostStatusInstallingInProgress, HostStatusError},
		DestinationState: HostStatusError,
		PostTransition:   th.PostCancelInstallation,
	})

	// Reset host
	sm.AddTransition(stateswitch.TransitionRule{
		TransitionType:   TransitionTypeResetHost,
		SourceStates:     []stateswitch.State{HostStatusError},
		DestinationState: HostStatusResetting,
		PostTransition:   th.PostResetHost,
	})

	// Install host
	sm.AddTransition(stateswitch.TransitionRule{
		TransitionType:   TransitionTypeInstallHost,
		Condition:        th.IsValidRoleForInstallation,
		SourceStates:     []stateswitch.State{HostStatusKnown},
		DestinationState: HostStatusInstalling,
		PostTransition:   th.PostInstallHost,
	})

	// Install disabled host will not do anything
	sm.AddTransition(stateswitch.TransitionRule{
		TransitionType:   TransitionTypeInstallHost,
		SourceStates:     []stateswitch.State{HostStatusDisabled},
		DestinationState: HostStatusDisabled,
	})

	// Disable host
	sm.AddTransition(stateswitch.TransitionRule{
		TransitionType: TransitionTypeDisableHost,
		SourceStates: []stateswitch.State{
			HostStatusDisconnected,
			HostStatusDiscovering,
			HostStatusInsufficient,
			HostStatusKnown,
		},
		DestinationState: HostStatusDisabled,
		PostTransition:   th.PostDisableHost,
	})

	// Enable host
	sm.AddTransition(stateswitch.TransitionRule{
		TransitionType: TransitionTypeEnableHost,
		SourceStates: []stateswitch.State{
			HostStatusDisabled,
		},
		DestinationState: HostStatusDiscovering,
		PostTransition:   th.PostEnableHost,
	})

	// Refresh host
	sm.AddTransition(stateswitch.TransitionRule{
		TransitionType:   TransitionTypeRefresh,
		SourceStates:     []stateswitch.State{HostStatusDiscovering, HostStatusInsufficient, HostStatusKnown, HostStatusPendingForInput, HostStatusDisconnected},
		Condition:        stateswitch.Not(th.IsConnected),
		DestinationState: HostStatusDisconnected,
		PostTransition:   th.PostRefreshHost(statusInfoDisconnected),
	})

	sm.AddTransition(stateswitch.TransitionRule{
		TransitionType:   TransitionTypeRefresh,
		SourceStates:     []stateswitch.State{HostStatusDisconnected, HostStatusDiscovering},
		Condition:        stateswitch.And(th.IsConnected, stateswitch.Not(th.HasInventory)),
		DestinationState: HostStatusDiscovering,
		PostTransition:   th.PostRefreshHost(statusInfoDiscovering),
	})

	/*
	 * Validations is a slice of validation objects. Each validation is a pair of [Condition, category, printer].
	 * Validations.Condition() is a Condition object that performs "and" operation between all the Condition elements in Validations.
	 * Validations are used in 3 places:
	 * 1. In a transition with expectation that all conditions will be true (i.e Validations.Condition() == true).  This means that the validation passed.
	 *    It is used in the Condition part of a transition rule.
	 * 2. In a transition with expectation that at least one of the conditions will fail (i.e Validations.Condition() == false -> Not(Validations.Condition()) == true).
	 *    It is used in the Condition part of a transition rule.
	 * 3. In a transition with expectation that at least one of the conditions will fail (i.e Validations.Condition() == false -> Not(Validations.Condition()) == true).
	 *    In this case it is used in the PostTransition callback function.  The purpose is to gather the failed validation results, which are formatted strings grouped by category, and use them
	 *    for the status-info.
	 */
	var minRequiredHardwareValidations = statemachine.Validations{
		statemachine.Validation(th.HasMinMemory, "hardware", statemachine.Sprintf("Insufficient RAM requirements, expected: %s got %s", th.ExpectedPhysicalMemory, th.PhysicalMemory)),
		statemachine.Validation(th.HasMinValidDisks, "hardware",
			statemachine.Sprintf("Insufficient number of disks with required size, expected at least 1 not removable, not readonly disk of size more than %d  bytes", th.ExpectedMinDiskSize)),
		statemachine.Validation(th.HasMinCpuCores, "hardware", statemachine.Sprintf("Insufficient CPU cores, expected: %d got %d", th.ExpectedCoreCount, th.CoreCount)),
	}

	var sufficientInputValidations = statemachine.Validations{
		statemachine.Validation(th.IsMachineCidrDefined, "network", statemachine.Sprintf("Machine network CIDR for cluster is missing, The machine network is set by configuring the API-VIP or Ingress-VIP")),
		statemachine.Validation(th.IsRoleDefined, "role", statemachine.Sprintf("Role is not defined")),
	}

	var sufficientForInstallValidations = statemachine.Validations{
		statemachine.Validation(th.HasMemoryForRole, "hardware", statemachine.Sprintf("Insufficient RAM requirements, expected: %s got %s", th.ExpectedPhysicalMemory, th.PhysicalMemory)),
		statemachine.Validation(th.HasValidDisksForRole, "hardware",
			statemachine.Sprintf("Insufficient number of disks with required size, expected at least 1 not removable, not readonly disk of size more than %d  bytes", th.ExpectedMinDiskSize)),
		statemachine.Validation(th.HasCpuCoresForRole, "hardware", statemachine.Sprintf("Insufficient CPU cores, expected: %d got %d", th.ExpectedCoreCount, th.CoreCount)),
		statemachine.Validation(th.BelongsToMachineCIDR, "network",
			statemachine.Sprintf("Host does not belong to the machine network cidr %s.  The machine network is set by configuring the API-VIP or Ingress-VIP", th.MachineNetworkCidr)),
		statemachine.Validation(th.IsHostnameUnique, "hardware", statemachine.Sprintf("Hostname %s is not unique in cluster", th.Hostname)),
	}

	// In order for this transition to be fired at least one of the validations in minRequiredHardwareValidations must fail.
	// This transition handles the case that a host does not pass minimum hardware requirements for any of the roles
	sm.AddTransition(stateswitch.TransitionRule{
		TransitionType: TransitionTypeRefresh,
		SourceStates:   []stateswitch.State{HostStatusDisconnected, HostStatusDiscovering, HostStatusInsufficient},
		Condition: stateswitch.And(th.IsConnected, th.HasInventory,
			stateswitch.Not(minRequiredHardwareValidations.Condition())),
		DestinationState: HostStatusInsufficient,
		PostTransition:   statemachine.MakePostValidation(minRequiredHardwareValidations, th.PostValidationFailure),
	})

	// In order for this transition to be fired at least one of the validations in sufficientInputValidations must fail.
	// This transition handles the case that there is missing input that has to be provided from a user or other external means
	sm.AddTransition(stateswitch.TransitionRule{
		TransitionType: TransitionTypeRefresh,
		SourceStates:   []stateswitch.State{HostStatusDisconnected, HostStatusDiscovering, HostStatusInsufficient, HostStatusKnown, HostStatusPendingForInput},
		Condition: stateswitch.And(th.IsConnected, th.HasInventory,
			minRequiredHardwareValidations.Condition(),
			stateswitch.Not(sufficientInputValidations.Condition())),
		DestinationState: HostStatusPendingForInput,
		PostTransition:   statemachine.MakePostValidation(sufficientInputValidations, th.PostValidationFailure),
	})

	// In order for this transition to be fired at least one of the validations in sufficientForInstallValidations must fail.
	// This transition handles the case that one of the required validations that are required in order for the host
	// to be in known state (ready for installation) has failed
	sm.AddTransition(stateswitch.TransitionRule{
		TransitionType: TransitionTypeRefresh,
		SourceStates:   []stateswitch.State{HostStatusDisconnected, HostStatusInsufficient, HostStatusPendingForInput, HostStatusDiscovering, HostStatusKnown},
		Condition: stateswitch.And(th.IsConnected, th.HasInventory,
			minRequiredHardwareValidations.Condition(),
			sufficientInputValidations.Condition(),
			stateswitch.Not(sufficientForInstallValidations.Condition())),
		DestinationState: HostStatusInsufficient,
		PostTransition:   statemachine.MakePostValidation(sufficientForInstallValidations, th.PostValidationFailure),
	})

	// This transition is fired when all validations pass
	sm.AddTransition(stateswitch.TransitionRule{
		TransitionType: TransitionTypeRefresh,
		SourceStates:   []stateswitch.State{HostStatusDisconnected, HostStatusInsufficient, HostStatusPendingForInput, HostStatusDiscovering, HostStatusKnown},
		Condition: stateswitch.And(th.IsConnected, th.HasInventory,
			minRequiredHardwareValidations.Condition(),
			sufficientInputValidations.Condition(),
			sufficientForInstallValidations.Condition()),
		DestinationState: HostStatusKnown,
		PostTransition:   th.PostRefreshHost(""),
	})
	return sm
}

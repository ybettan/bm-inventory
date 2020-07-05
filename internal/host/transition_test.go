package host

import (
	"context"
	"encoding/json"
	"time"

	"github.com/onsi/gomega/types"

	"github.com/filanov/bm-inventory/internal/common"

	"github.com/filanov/bm-inventory/internal/hardware"

	"github.com/go-openapi/swag"

	. "github.com/onsi/gomega"

	"github.com/filanov/bm-inventory/models"

	"github.com/go-openapi/strfmt"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	. "github.com/onsi/ginkgo"
)

func createValidatorCfg() hardware.ValidatorCfg {
	return hardware.ValidatorCfg{
		MinCPUCores:       2,
		MinCPUCoresWorker: 2,
		MinCPUCoresMaster: 4,
		MinDiskSizeGib:    120,
		MinRamGib:         8,
		MinRamGibWorker:   8,
		MinRamGibMaster:   16,
	}
}

var _ = Describe("RegisterHost", func() {
	var (
		ctx               = context.Background()
		hapi              API
		db                *gorm.DB
		hostId, clusterId strfmt.UUID
	)

	BeforeEach(func() {
		db = prepareDB()
		hapi = NewManager(getTestLog(), db, nil, nil, nil, nil, createValidatorCfg())
		hostId = strfmt.UUID(uuid.New().String())
		clusterId = strfmt.UUID(uuid.New().String())
	})

	It("register_new", func() {
		Expect(hapi.RegisterHost(ctx, &models.Host{ID: &hostId, ClusterID: clusterId, DiscoveryAgentVersion: "v1.0.1"})).ShouldNot(HaveOccurred())
		h := getHost(hostId, clusterId, db)
		Expect(swag.StringValue(h.Status)).Should(Equal(HostStatusDiscovering))
		Expect(h.DiscoveryAgentVersion).To(Equal("v1.0.1"))
	})

	Context("register during installation put host in error", func() {
		tests := []struct {
			name     string
			srcState string
		}{
			{
				name:     "discovering",
				srcState: HostStatusInstalling,
			},
			{
				name:     "insufficient",
				srcState: HostStatusInstallingInProgress,
			},
		}

		AfterEach(func() {
			h := getHost(hostId, clusterId, db)
			Expect(swag.StringValue(h.Status)).Should(Equal(HostStatusError))
			Expect(h.Role).Should(Equal(models.HostRoleMaster))
			Expect(h.HardwareInfo).Should(Equal(defaultHwInfo))
			Expect(h.StatusInfo).NotTo(BeNil())
		})

		for i := range tests {
			t := tests[i]

			It(t.name, func() {
				Expect(db.Create(&models.Host{
					ID:           &hostId,
					ClusterID:    clusterId,
					Role:         models.HostRoleMaster,
					HardwareInfo: defaultHwInfo,
					Status:       swag.String(t.srcState),
				}).Error).ShouldNot(HaveOccurred())

				Expect(hapi.RegisterHost(ctx, &models.Host{
					ID:        &hostId,
					ClusterID: clusterId,
					Status:    swag.String(t.srcState),
				})).ShouldNot(HaveOccurred())
			})
		}
	})

	Context("host already exists register success", func() {
		discoveryAgentVersion := "v2.0.5"
		tests := []struct {
			name     string
			srcState string
		}{
			{
				name:     "discovering",
				srcState: HostStatusDiscovering,
			},
			{
				name:     "insufficient",
				srcState: HostStatusInsufficient,
			},
			{
				name:     "disconnected",
				srcState: HostStatusDisconnected,
			},
			{
				name:     "known",
				srcState: HostStatusKnown,
			},
		}

		AfterEach(func() {
			h := getHost(hostId, clusterId, db)
			Expect(swag.StringValue(h.Status)).Should(Equal(HostStatusDiscovering))
			Expect(h.Role).Should(Equal(models.HostRoleMaster))
			Expect(h.HardwareInfo).Should(Equal(""))
			Expect(h.DiscoveryAgentVersion).To(Equal(discoveryAgentVersion))
		})

		for i := range tests {
			t := tests[i]

			It(t.name, func() {
				Expect(db.Create(&models.Host{
					ID:           &hostId,
					ClusterID:    clusterId,
					Role:         models.HostRoleMaster,
					HardwareInfo: defaultHwInfo,
					Status:       swag.String(t.srcState),
				}).Error).ShouldNot(HaveOccurred())

				Expect(hapi.RegisterHost(ctx, &models.Host{
					ID:                    &hostId,
					ClusterID:             clusterId,
					Status:                swag.String(t.srcState),
					DiscoveryAgentVersion: discoveryAgentVersion,
				})).ShouldNot(HaveOccurred())
			})
		}
	})

	Context("host already exist registration fail", func() {
		tests := []struct {
			name        string
			srcState    string
			targetState string
		}{
			{
				name:     "disabled",
				srcState: HostStatusDisabled,
			},
			{
				name:     "error",
				srcState: HostStatusError,
			},
			{
				name:     "installed",
				srcState: HostStatusInstalled,
			},
		}

		for i := range tests {
			t := tests[i]

			It(t.name, func() {
				Expect(db.Create(&models.Host{
					ID:           &hostId,
					ClusterID:    clusterId,
					Role:         models.HostRoleMaster,
					HardwareInfo: defaultHwInfo,
					Status:       swag.String(t.srcState),
				}).Error).ShouldNot(HaveOccurred())

				Expect(hapi.RegisterHost(ctx, &models.Host{
					ID:        &hostId,
					ClusterID: clusterId,
					Status:    swag.String(t.srcState),
				})).Should(HaveOccurred())

				h := getHost(hostId, clusterId, db)
				Expect(swag.StringValue(h.Status)).Should(Equal(t.srcState))
				Expect(h.Role).Should(Equal(models.HostRoleMaster))
				Expect(h.HardwareInfo).Should(Equal(defaultHwInfo))
			})
		}
	})

	Context("register after reboot", func() {
		tests := []struct {
			name     string
			srcState string
			progress models.HostProgress
		}{
			{
				name:     "host in reboot",
				srcState: HostStatusInstallingInProgress,
				progress: models.HostProgress{
					CurrentStage: models.HostStageRebooting,
				},
			},
		}

		AfterEach(func() {
			h := getHost(hostId, clusterId, db)
			Expect(swag.StringValue(h.Status)).Should(Equal(models.HostStatusInstallingPendingUserAction))
			Expect(h.Role).Should(Equal(models.HostRoleMaster))
			Expect(h.HardwareInfo).Should(Equal(defaultHwInfo))
			Expect(h.StatusInfo).NotTo(BeNil())
		})

		for i := range tests {
			t := tests[i]

			It(t.name, func() {
				Expect(db.Create(&models.Host{
					ID:           &hostId,
					ClusterID:    clusterId,
					Role:         models.HostRoleMaster,
					HardwareInfo: defaultHwInfo,
					Status:       swag.String(t.srcState),
					Progress:     &t.progress,
				}).Error).ShouldNot(HaveOccurred())

				Expect(hapi.RegisterHost(ctx, &models.Host{
					ID:        &hostId,
					ClusterID: clusterId,
					Status:    swag.String(t.srcState),
				})).ShouldNot(HaveOccurred())
			})
		}
	})

	AfterEach(func() {
		_ = db.Close()
	})
})

var _ = Describe("HostInstallationFailed", func() {
	var (
		ctx               = context.Background()
		hapi              API
		db                *gorm.DB
		hostId, clusterId strfmt.UUID
		host              models.Host
	)

	BeforeEach(func() {
		db = prepareDB()
		hapi = NewManager(getTestLog(), db, nil, nil, nil, nil, createValidatorCfg())
		hostId = strfmt.UUID(uuid.New().String())
		clusterId = strfmt.UUID(uuid.New().String())
		host = getTestHost(hostId, clusterId, "")
		host.Status = swag.String(HostStatusInstalling)
		Expect(db.Create(&host).Error).ShouldNot(HaveOccurred())
	})

	It("handle_installation_error", func() {
		Expect(hapi.HandleInstallationFailure(ctx, &host)).ShouldNot(HaveOccurred())
		h := getHost(hostId, clusterId, db)
		Expect(swag.StringValue(h.Status)).Should(Equal(HostStatusError))
		Expect(swag.StringValue(h.StatusInfo)).Should(Equal("installation command failed"))
	})

	AfterEach(func() {
		_ = db.Close()
	})
})

var _ = Describe("Install", func() {
	var (
		ctx               = context.Background()
		hapi              API
		db                *gorm.DB
		hostId, clusterId strfmt.UUID
		host              models.Host
	)

	BeforeEach(func() {
		db = prepareDB()
		hapi = NewManager(getTestLog(), db, nil, nil, nil, nil, createValidatorCfg())
		hostId = strfmt.UUID(uuid.New().String())
		clusterId = strfmt.UUID(uuid.New().String())
	})

	Context("install host", func() {
		success := func(reply error) {
			Expect(reply).To(BeNil())
			h := getHost(hostId, clusterId, db)
			Expect(*h.Status).Should(Equal(HostStatusInstalling))
			Expect(*h.StatusInfo).Should(Equal(statusInfoInstalling))
		}

		failure := func(reply error) {
			Expect(reply).To(HaveOccurred())
		}

		noChange := func(reply error) {
			Expect(reply).To(BeNil())
			h := getHost(hostId, clusterId, db)
			Expect(*h.Status).Should(Equal(HostStatusDisabled))
		}

		tests := []struct {
			name       string
			srcState   string
			role       models.HostRole
			validation func(error)
		}{
			{
				name:       "known with role worker",
				srcState:   HostStatusKnown,
				role:       models.HostRoleWorker,
				validation: success,
			},
			{
				name:       "known with role master",
				srcState:   HostStatusKnown,
				role:       models.HostRoleMaster,
				validation: success,
			},
			{
				name:       "known without role",
				srcState:   HostStatusKnown,
				validation: failure,
			},
			{
				name:       "disabled nothing change",
				srcState:   HostStatusDisabled,
				role:       models.HostRoleMaster,
				validation: noChange,
			},
			{
				name:       "disconnected",
				srcState:   HostStatusDisconnected,
				role:       models.HostRoleMaster,
				validation: failure,
			},
			{
				name:       "discovering",
				srcState:   HostStatusDiscovering,
				role:       models.HostRoleMaster,
				validation: failure,
			},
			{
				name:       "error",
				srcState:   HostStatusError,
				role:       models.HostRoleMaster,
				validation: failure,
			},
			{
				name:       "installed",
				srcState:   HostStatusInstalled,
				role:       models.HostRoleMaster,
				validation: failure,
			},
			{
				name:       "installing",
				srcState:   HostStatusInstalling,
				role:       models.HostRoleMaster,
				validation: failure,
			},
			{
				name:       "in-progress",
				srcState:   HostStatusInstallingInProgress,
				role:       models.HostRoleMaster,
				validation: failure,
			},
			{
				name:       "insufficient",
				srcState:   HostStatusInsufficient,
				role:       models.HostRoleMaster,
				validation: failure,
			},
			{
				name:       "resetting",
				srcState:   HostStatusResetting,
				role:       models.HostRoleMaster,
				validation: failure,
			},
		}

		for i := range tests {
			t := tests[i]
			It(t.name, func() {
				host = getTestHost(hostId, clusterId, t.srcState)
				host.Role = t.role
				Expect(db.Create(&host).Error).ShouldNot(HaveOccurred())
				t.validation(hapi.Install(ctx, &host, nil))
			})
		}
	})

	Context("install with transaction", func() {
		BeforeEach(func() {
			host = getTestHost(hostId, clusterId, HostStatusKnown)
			host.Role = models.HostRoleMaster
			host.StatusInfo = swag.String("known")
			Expect(db.Create(&host).Error).ShouldNot(HaveOccurred())
		})

		It("success", func() {
			tx := db.Begin()
			Expect(tx.Error).To(BeNil())
			Expect(hapi.Install(ctx, &host, tx)).ShouldNot(HaveOccurred())
			Expect(tx.Commit().Error).ShouldNot(HaveOccurred())
			h := getHost(hostId, clusterId, db)
			Expect(*h.Status).Should(Equal(HostStatusInstalling))
			Expect(*h.StatusInfo).Should(Equal(statusInfoInstalling))
		})

		It("rollback transition", func() {
			tx := db.Begin()
			Expect(tx.Error).To(BeNil())
			Expect(hapi.Install(ctx, &host, tx)).ShouldNot(HaveOccurred())
			Expect(tx.Rollback().Error).ShouldNot(HaveOccurred())
			h := getHost(hostId, clusterId, db)
			Expect(*h.Status).Should(Equal(HostStatusKnown))
			Expect(*h.StatusInfo).Should(Equal("known"))
		})
	})

	AfterEach(func() {
		_ = db.Close()
	})
})

var _ = Describe("Disable", func() {
	var (
		ctx               = context.Background()
		hapi              API
		db                *gorm.DB
		hostId, clusterId strfmt.UUID
		host              models.Host
	)

	BeforeEach(func() {
		db = prepareDB()
		hapi = NewManager(getTestLog(), db, nil, nil, nil, nil, createValidatorCfg())
		hostId = strfmt.UUID(uuid.New().String())
		clusterId = strfmt.UUID(uuid.New().String())
	})

	Context("disable host", func() {
		var srcState string
		success := func(reply error) {
			Expect(reply).To(BeNil())
			h := getHost(hostId, clusterId, db)
			Expect(*h.Status).Should(Equal(HostStatusDisabled))
			Expect(*h.StatusInfo).Should(Equal(statusInfoDisabled))
		}

		failure := func(reply error) {
			Expect(reply).To(HaveOccurred())
			h := getHost(hostId, clusterId, db)
			Expect(*h.Status).Should(Equal(srcState))
		}

		tests := []struct {
			name       string
			srcState   string
			validation func(error)
		}{
			{
				name:       "known",
				srcState:   HostStatusKnown,
				validation: success,
			},
			{
				name:       "disabled nothing change",
				srcState:   HostStatusDisabled,
				validation: failure,
			},
			{
				name:       "disconnected",
				srcState:   HostStatusDisconnected,
				validation: success,
			},
			{
				name:       "discovering",
				srcState:   HostStatusDiscovering,
				validation: success,
			},
			{
				name:       "error",
				srcState:   HostStatusError,
				validation: failure,
			},
			{
				name:       "installed",
				srcState:   HostStatusInstalled,
				validation: failure,
			},
			{
				name:       "installing",
				srcState:   HostStatusInstalling,
				validation: failure,
			},
			{
				name:       "in-progress",
				srcState:   HostStatusInstallingInProgress,
				validation: failure,
			},
			{
				name:       "insufficient",
				srcState:   HostStatusInsufficient,
				validation: success,
			},
			{
				name:       "resetting",
				srcState:   HostStatusResetting,
				validation: failure,
			},
		}

		for i := range tests {
			t := tests[i]
			It(t.name, func() {
				srcState = t.srcState
				host = getTestHost(hostId, clusterId, srcState)
				Expect(db.Create(&host).Error).ShouldNot(HaveOccurred())
				t.validation(hapi.DisableHost(ctx, &host))
			})
		}
	})

	AfterEach(func() {
		_ = db.Close()
	})
})

var _ = Describe("Enable", func() {
	var (
		ctx               = context.Background()
		hapi              API
		db                *gorm.DB
		hostId, clusterId strfmt.UUID
		host              models.Host
	)

	BeforeEach(func() {
		db = prepareDB()
		hapi = NewManager(getTestLog(), db, nil, nil, nil, nil, createValidatorCfg())
		hostId = strfmt.UUID(uuid.New().String())
		clusterId = strfmt.UUID(uuid.New().String())
	})

	Context("enable host", func() {
		var srcState string
		success := func(reply error) {
			Expect(reply).To(BeNil())
			h := getHost(hostId, clusterId, db)
			Expect(*h.Status).Should(Equal(HostStatusDiscovering))
			Expect(*h.StatusInfo).Should(Equal(statusInfoDiscovering))
			Expect(h.HardwareInfo).Should(Equal(""))
		}

		failure := func(reply error) {
			Expect(reply).Should(HaveOccurred())
			h := getHost(hostId, clusterId, db)
			Expect(*h.Status).Should(Equal(srcState))
			Expect(h.HardwareInfo).Should(Equal(defaultHwInfo))
		}

		tests := []struct {
			name       string
			srcState   string
			validation func(error)
		}{
			{
				name:       "known",
				srcState:   HostStatusKnown,
				validation: failure,
			},
			{
				name:       "disabled to enable",
				srcState:   HostStatusDisabled,
				validation: success,
			},
			{
				name:       "disconnected",
				srcState:   HostStatusDisconnected,
				validation: failure,
			},
			{
				name:       "discovering",
				srcState:   HostStatusDiscovering,
				validation: failure,
			},
			{
				name:       "error",
				srcState:   HostStatusError,
				validation: failure,
			},
			{
				name:       "installed",
				srcState:   HostStatusInstalled,
				validation: failure,
			},
			{
				name:       "installing",
				srcState:   HostStatusInstalling,
				validation: failure,
			},
			{
				name:       "in-progress",
				srcState:   HostStatusInstallingInProgress,
				validation: failure,
			},
			{
				name:       "insufficient",
				srcState:   HostStatusInsufficient,
				validation: failure,
			},
			{
				name:       "resetting",
				srcState:   HostStatusResetting,
				validation: failure,
			},
		}

		for i := range tests {
			t := tests[i]
			It(t.name, func() {
				srcState = t.srcState
				host = getTestHost(hostId, clusterId, srcState)
				host.HardwareInfo = defaultHwInfo
				Expect(db.Create(&host).Error).ShouldNot(HaveOccurred())
				t.validation(hapi.EnableHost(ctx, &host))
			})
		}
	})

	AfterEach(func() {
		_ = db.Close()
	})
})

type statusInfoChecker interface {
	check(statusInfo *string)
}

type valueChecker struct {
	value string
}

func (v *valueChecker) check(value *string) {
	if value == nil {
		Expect(v.value).To(Equal(""))
	} else {
		Expect(*value).To(Equal(v.value))
	}
}

func makeValueChecker(value string) statusInfoChecker {
	return &valueChecker{value: value}
}

type jsonChecker struct {
	patterns map[string][]string
}

func (j *jsonChecker) check(valuesStr *string) {
	Expect(valuesStr).ToNot(BeNil())
	valuesMap := make(map[string][]string)
	Expect(json.Unmarshal([]byte(*valuesStr), &valuesMap)).ToNot(HaveOccurred())
	for key, patternsSlice := range j.patterns {
		valuesSlice, ok := valuesMap[key]
		Expect(len(valuesSlice)).To(Equal(len(patternsSlice)))
		Expect(ok).To(BeTrue())
		matchers := make([]types.GomegaMatcher, 0)
		for _, p := range patternsSlice {
			matchers = append(matchers, MatchRegexp(p))
		}
		for _, v := range valuesSlice {
			Expect(v).To(Or(matchers...))
		}
	}
}

func makeJsonChecker(patterns map[string][]string) statusInfoChecker {
	return &jsonChecker{patterns: patterns}
}

var _ = Describe("Refresh Host", func() {
	var (
		ctx               = context.Background()
		hapi              API
		db                *gorm.DB
		hostId, clusterId strfmt.UUID
		host              models.Host
		cluster           common.Cluster
	)

	BeforeEach(func() {
		db = prepareDB()
		hapi = NewManager(getTestLog(), db, nil, nil, nil, nil, createValidatorCfg())
		hostId = strfmt.UUID(uuid.New().String())
		clusterId = strfmt.UUID(uuid.New().String())
	})
	Context("All transitions", func() {
		var srcState string
		tests := []struct {
			name               string
			srcState           string
			inventory          string
			role               string
			machineNetworkCidr string
			checkedInAt        strfmt.DateTime
			dstState           string
			statusInfoChecker  statusInfoChecker
			errorExpected      bool
		}{
			{
				name:              "discovering to disconnected",
				srcState:          HostStatusDiscovering,
				dstState:          HostStatusDisconnected,
				statusInfoChecker: makeValueChecker(statusInfoDisconnected),
				errorExpected:     false,
			},
			{
				name:              "insufficient to disconnected",
				srcState:          HostStatusInsufficient,
				dstState:          HostStatusDisconnected,
				statusInfoChecker: makeValueChecker(statusInfoDisconnected),
				errorExpected:     false,
			},
			{
				name:              "known to disconnected",
				srcState:          HostStatusKnown,
				dstState:          HostStatusDisconnected,
				statusInfoChecker: makeValueChecker(statusInfoDisconnected),
				errorExpected:     false,
			},
			{
				name:              "pending to disconnected",
				srcState:          HostStatusPendingForInput,
				dstState:          HostStatusDisconnected,
				statusInfoChecker: makeValueChecker(statusInfoDisconnected),
				errorExpected:     false,
			},
			{
				name:              "disconnected to disconnected",
				srcState:          HostStatusDisconnected,
				dstState:          HostStatusDisconnected,
				statusInfoChecker: makeValueChecker(statusInfoDisconnected),
				errorExpected:     false,
			},
			{
				name:              "disconnected to discovering",
				checkedInAt:       strfmt.DateTime(time.Now()),
				srcState:          HostStatusDisconnected,
				dstState:          HostStatusDiscovering,
				statusInfoChecker: makeValueChecker(statusInfoDiscovering),
				errorExpected:     false,
			},
			{
				name:              "discovering to discovering",
				checkedInAt:       strfmt.DateTime(time.Now()),
				srcState:          HostStatusDiscovering,
				dstState:          HostStatusDiscovering,
				statusInfoChecker: makeValueChecker(statusInfoDiscovering),
				errorExpected:     false,
			},
			{
				name:        "disconnected to insufficient (1)",
				checkedInAt: strfmt.DateTime(time.Now()),
				srcState:    HostStatusDisconnected,
				dstState:    HostStatusInsufficient,
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"hardware": {
							"^Insufficient RAM requirements.*$",
							"^Insufficient number of disks with required size.*$",
						},
					}),
				inventory:     insufficientHWInventory(),
				errorExpected: false,
			},
			{
				name:        "insufficient to insufficient (1)",
				checkedInAt: strfmt.DateTime(time.Now()),
				srcState:    HostStatusInsufficient,
				dstState:    HostStatusInsufficient,
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"hardware": {
							"^Insufficient RAM requirements.*$",
							"^Insufficient number of disks with required size.*$",
						},
					}),
				inventory:     insufficientHWInventory(),
				errorExpected: false,
			},
			{
				name:        "discovering to insufficient (1)",
				checkedInAt: strfmt.DateTime(time.Now()),
				srcState:    HostStatusDiscovering,
				dstState:    HostStatusInsufficient,
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"hardware": {
							"^Insufficient RAM requirements.*$",
							"^Insufficient number of disks with required size.*$",
						},
					}),
				inventory:     insufficientHWInventory(),
				errorExpected: false,
			},
			{
				name:              "pending to insufficient (1)",
				checkedInAt:       strfmt.DateTime(time.Now()),
				srcState:          HostStatusPendingForInput,
				dstState:          HostStatusPendingForInput,
				statusInfoChecker: makeValueChecker(""),
				inventory:         insufficientHWInventory(),
				errorExpected:     true,
			},
			{
				name:              "known to insufficient (1)",
				checkedInAt:       strfmt.DateTime(time.Now()),
				srcState:          HostStatusKnown,
				dstState:          HostStatusKnown,
				statusInfoChecker: makeValueChecker(""),
				inventory:         insufficientHWInventory(),
				errorExpected:     true,
			},
			{
				name:        "disconnected to pending",
				checkedInAt: strfmt.DateTime(time.Now()),
				srcState:    HostStatusDisconnected,
				dstState:    HostStatusPendingForInput,
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"network": {
							"^Machine network CIDR for cluster is missing, The machine network is set by configuring the API-VIP or Ingress-VIP$",
						},
						"role": {
							"^Role is not defined$",
						},
					}),
				inventory:     workerInventory(),
				errorExpected: false,
			},
			{
				name:               "discovering to pending",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusDiscovering,
				dstState:           HostStatusPendingForInput,
				machineNetworkCidr: "5.6.7.0/24",
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"role": {
							"^Role is not defined$",
						},
					}),
				inventory:     workerInventory(),
				errorExpected: false,
			},
			{
				name:               "insufficient to pending",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusInsufficient,
				dstState:           HostStatusPendingForInput,
				machineNetworkCidr: "5.6.7.0/24",
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"role": {
							"^Role is not defined$",
						},
					}),
				inventory:     workerInventory(),
				errorExpected: false,
			},
			{
				name:        "known to pending",
				checkedInAt: strfmt.DateTime(time.Now()),
				srcState:    HostStatusKnown,
				dstState:    HostStatusPendingForInput,
				role:        "worker",
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"network": {
							"^Machine network CIDR for cluster is missing, The machine network is set by configuring the API-VIP or Ingress-VIP$",
						},
					}),
				inventory:     workerInventory(),
				errorExpected: false,
			},
			{
				name:        "pending to pending",
				checkedInAt: strfmt.DateTime(time.Now()),
				srcState:    HostStatusPendingForInput,
				dstState:    HostStatusPendingForInput,
				role:        "worker",
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"network": {
							"^Machine network CIDR for cluster is missing, The machine network is set by configuring the API-VIP or Ingress-VIP$",
						},
					}),
				inventory:     workerInventory(),
				errorExpected: false,
			},
			{
				name:               "disconnected to insufficient (2)",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusDisconnected,
				dstState:           HostStatusInsufficient,
				machineNetworkCidr: "5.6.7.0/24",
				role:               "worker",
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"network": {
							"^Host does not belong to the machine network cidr .*$",
						},
					}),
				inventory:     workerInventory(),
				errorExpected: false,
			},
			{
				name:               "discovering to insufficient (2)",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusDiscovering,
				dstState:           HostStatusInsufficient,
				machineNetworkCidr: "5.6.7.0/24",
				role:               "master",
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"network": {
							"^Host does not belong to the machine network cidr .*$",
						},
						"hardware": {
							"^Insufficient CPU cores, expected: .*",
							"^Insufficient RAM requirements, expected: .*",
						},
					}),
				inventory:     workerInventory(),
				errorExpected: false,
			},
			{
				name:               "insufficient to insufficient (2)",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusInsufficient,
				dstState:           HostStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				role:               "master",
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"hardware": {
							"^Insufficient CPU cores, expected: .*",
							"^Insufficient RAM requirements, expected: .*",
						},
					}),
				inventory:     workerInventory(),
				errorExpected: false,
			},
			{
				name:               "pending to insufficient (2)",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusPendingForInput,
				dstState:           HostStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				role:               "master",
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"hardware": {
							"^Insufficient CPU cores, expected: .*",
							"^Insufficient RAM requirements, expected: .*",
						},
					}),
				inventory:     workerInventory(),
				errorExpected: false,
			},
			{
				name:               "known to insufficient (2)",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusKnown,
				dstState:           HostStatusInsufficient,
				machineNetworkCidr: "5.6.7.0/24",
				role:               "master",
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"network": {
							"^Host does not belong to the machine network cidr .*$",
						},
					}),
				inventory:     masterInventory(),
				errorExpected: false,
			},
			{
				name:               "insufficient to insufficient (2)",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusInsufficient,
				dstState:           HostStatusInsufficient,
				machineNetworkCidr: "5.6.7.0/24",
				role:               "master",
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"network": {
							"^Host does not belong to the machine network cidr .*$",
						},
					}),
				inventory:     masterInventory(),
				errorExpected: false,
			},
			{
				name:               "discovering to known",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusDiscovering,
				dstState:           HostStatusKnown,
				machineNetworkCidr: "1.2.3.0/24",
				role:               "master",
				statusInfoChecker:  makeValueChecker(""),
				inventory:          masterInventory(),
				errorExpected:      false,
			},
			{
				name:               "insufficient to known",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusInsufficient,
				dstState:           HostStatusKnown,
				machineNetworkCidr: "1.2.3.0/24",
				role:               "worker",
				statusInfoChecker:  makeValueChecker(""),
				inventory:          masterInventory(),
				errorExpected:      false,
			},
			{
				name:               "pending to known",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusPendingForInput,
				dstState:           HostStatusKnown,
				machineNetworkCidr: "1.2.3.0/24",
				role:               "worker",
				statusInfoChecker:  makeValueChecker(""),
				inventory:          masterInventory(),
				errorExpected:      false,
			},
			{
				name:               "known to known",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusKnown,
				dstState:           HostStatusKnown,
				machineNetworkCidr: "1.2.3.0/24",
				role:               "master",
				statusInfoChecker:  makeValueChecker(""),
				inventory:          masterInventory(),
				errorExpected:      false,
			},
			{
				name:               "known to known with unexpected role",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusKnown,
				dstState:           HostStatusKnown,
				machineNetworkCidr: "1.2.3.0/24",
				role:               "kuku",
				statusInfoChecker:  makeValueChecker(""),
				inventory:          masterInventory(),
				errorExpected:      true,
			},
		}

		for i := range tests {
			t := tests[i]
			It(t.name, func() {
				srcState = t.srcState
				host = getTestHost(hostId, clusterId, srcState)
				host.Inventory = t.inventory
				host.Role = models.HostRole(t.role)
				host.CheckedInAt = t.checkedInAt
				Expect(db.Create(&host).Error).ShouldNot(HaveOccurred())
				cluster = getTestCluster(clusterId, t.machineNetworkCidr)
				Expect(db.Create(&cluster).Error).ToNot(HaveOccurred())
				err := hapi.RefreshStatus(ctx, &host, db)
				if t.errorExpected {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).ToNot(HaveOccurred())
				}
				var resultHost models.Host
				Expect(db.Take(&resultHost, "id = ? and cluster_id = ?", hostId.String(), clusterId.String()).Error).ToNot(HaveOccurred())
				Expect(resultHost.Role).To(Equal(models.HostRole(t.role)))
				Expect(resultHost.Status).To(Equal(&t.dstState))
				t.statusInfoChecker.check(resultHost.StatusInfo)
			})
		}
	})
	Context("Unique hostname", func() {
		var srcState string
		var otherHostID strfmt.UUID

		BeforeEach(func() {
			otherHostID = strfmt.UUID(uuid.New().String())
		})

		tests := []struct {
			name                   string
			srcState               string
			inventory              string
			role                   string
			machineNetworkCidr     string
			checkedInAt            strfmt.DateTime
			dstState               string
			requestedHostname      string
			otherState             string
			otherRequestedHostname string
			otherInventory         string
			statusInfoChecker      statusInfoChecker
			errorExpected          bool
		}{
			{
				name:               "insufficient to known",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusInsufficient,
				dstState:           HostStatusKnown,
				machineNetworkCidr: "1.2.3.0/24",
				role:               "worker",
				statusInfoChecker:  makeValueChecker(""),
				inventory:          masterInventoryWithHostname("first"),
				otherState:         HostStatusInsufficient,
				otherInventory:     masterInventoryWithHostname("second"),
				errorExpected:      false,
			},
			{
				name:               "insufficient to insufficient (same hostname) 1",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusInsufficient,
				dstState:           HostStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				role:               "worker",
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"hardware": {
							"^Hostname first is not unique in cluster$",
						},
					}),
				inventory:      masterInventoryWithHostname("first"),
				otherState:     HostStatusInsufficient,
				otherInventory: masterInventoryWithHostname("first"),
				errorExpected:  false,
			},
			{
				name:               "insufficient to insufficient (same hostname) 2",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusInsufficient,
				dstState:           HostStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				role:               "worker",
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"hardware": {
							"^Hostname first is not unique in cluster$",
						},
					}),
				inventory:              masterInventoryWithHostname("first"),
				otherState:             HostStatusInsufficient,
				otherInventory:         masterInventoryWithHostname("second"),
				otherRequestedHostname: "first",
				errorExpected:          false,
			},
			{
				name:               "insufficient to insufficient (same hostname) 3",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusInsufficient,
				dstState:           HostStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				role:               "worker",
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"hardware": {
							"^Hostname second is not unique in cluster$",
						},
					}),
				inventory:         masterInventoryWithHostname("first"),
				requestedHostname: "second",
				otherState:        HostStatusInsufficient,
				otherInventory:    masterInventoryWithHostname("second"),
				errorExpected:     false,
			},
			{
				name:               "insufficient to insufficient (same hostname) 4",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusInsufficient,
				dstState:           HostStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				role:               "worker",
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"hardware": {
							"^Hostname third is not unique in cluster$",
						},
					}),
				inventory:              masterInventoryWithHostname("first"),
				requestedHostname:      "third",
				otherState:             HostStatusInsufficient,
				otherInventory:         masterInventoryWithHostname("second"),
				otherRequestedHostname: "third",
				errorExpected:          false,
			},
			{
				name:                   "insufficient to known 2",
				checkedInAt:            strfmt.DateTime(time.Now()),
				srcState:               HostStatusInsufficient,
				dstState:               HostStatusKnown,
				machineNetworkCidr:     "1.2.3.0/24",
				role:                   "worker",
				statusInfoChecker:      makeValueChecker(""),
				inventory:              masterInventoryWithHostname("first"),
				requestedHostname:      "third",
				otherState:             HostStatusInsufficient,
				otherInventory:         masterInventoryWithHostname("second"),
				otherRequestedHostname: "forth",
				errorExpected:          false,
			},

			{
				name:               "known to known",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusKnown,
				dstState:           HostStatusKnown,
				machineNetworkCidr: "1.2.3.0/24",
				role:               "worker",
				statusInfoChecker:  makeValueChecker(""),
				inventory:          masterInventoryWithHostname("first"),
				otherState:         HostStatusInsufficient,
				otherInventory:     masterInventoryWithHostname("second"),
				errorExpected:      false,
			},
			{
				name:               "known to insufficient (same hostname) 1",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusKnown,
				dstState:           HostStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				role:               "worker",
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"hardware": {
							"^Hostname first is not unique in cluster$",
						},
					}),
				inventory:      masterInventoryWithHostname("first"),
				otherState:     HostStatusInsufficient,
				otherInventory: masterInventoryWithHostname("first"),
				errorExpected:  false,
			},
			{
				name:               "known to insufficient (same hostname) 2",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusKnown,
				dstState:           HostStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				role:               "worker",
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"hardware": {
							"^Hostname first is not unique in cluster$",
						},
					}),
				inventory:              masterInventoryWithHostname("first"),
				otherState:             HostStatusInsufficient,
				otherInventory:         masterInventoryWithHostname("second"),
				otherRequestedHostname: "first",
				errorExpected:          false,
			},
			{
				name:               "known to insufficient (same hostname) 3",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusKnown,
				dstState:           HostStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				role:               "worker",
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"hardware": {
							"^Hostname second is not unique in cluster$",
						},
					}),
				inventory:         masterInventoryWithHostname("first"),
				requestedHostname: "second",
				otherState:        HostStatusInsufficient,
				otherInventory:    masterInventoryWithHostname("second"),
				errorExpected:     false,
			},
			{
				name:               "known to insufficient (same hostname) 4",
				checkedInAt:        strfmt.DateTime(time.Now()),
				srcState:           HostStatusKnown,
				dstState:           HostStatusInsufficient,
				machineNetworkCidr: "1.2.3.0/24",
				role:               "worker",
				statusInfoChecker: makeJsonChecker(
					map[string][]string{
						"hardware": {
							"^Hostname third is not unique in cluster$",
						},
					}),
				inventory:              masterInventoryWithHostname("first"),
				requestedHostname:      "third",
				otherState:             HostStatusInsufficient,
				otherInventory:         masterInventoryWithHostname("second"),
				otherRequestedHostname: "third",
				errorExpected:          false,
			},
			{
				name:                   "known to known 2",
				checkedInAt:            strfmt.DateTime(time.Now()),
				srcState:               HostStatusKnown,
				dstState:               HostStatusKnown,
				machineNetworkCidr:     "1.2.3.0/24",
				role:                   "worker",
				statusInfoChecker:      makeValueChecker(""),
				inventory:              masterInventoryWithHostname("first"),
				requestedHostname:      "third",
				otherState:             HostStatusInsufficient,
				otherInventory:         masterInventoryWithHostname("first"),
				otherRequestedHostname: "forth",
				errorExpected:          false,
			},
		}

		for i := range tests {
			t := tests[i]
			It(t.name, func() {
				srcState = t.srcState
				host = getTestHost(hostId, clusterId, srcState)
				host.Inventory = t.inventory
				host.Role = models.HostRole(t.role)
				host.CheckedInAt = t.checkedInAt
				host.RequestedHostname = t.requestedHostname
				Expect(db.Create(&host).Error).ShouldNot(HaveOccurred())
				otherHost := getTestHost(otherHostID, clusterId, t.otherState)
				otherHost.RequestedHostname = t.otherRequestedHostname
				otherHost.Inventory = t.otherInventory
				Expect(db.Create(&otherHost).Error).ShouldNot(HaveOccurred())
				cluster = getTestCluster(clusterId, t.machineNetworkCidr)
				Expect(db.Create(&cluster).Error).ToNot(HaveOccurred())
				err := hapi.RefreshStatus(ctx, &host, db)
				if t.errorExpected {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).ToNot(HaveOccurred())
				}
				var resultHost models.Host
				Expect(db.Take(&resultHost, "id = ? and cluster_id = ?", hostId.String(), clusterId.String()).Error).ToNot(HaveOccurred())
				Expect(resultHost.Role).To(Equal(models.HostRole(t.role)))
				Expect(resultHost.Status).To(Equal(&t.dstState))
				t.statusInfoChecker.check(resultHost.StatusInfo)
			})
		}
	})
	AfterEach(func() {
		_ = db.Close()
	})
})

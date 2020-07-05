package host

import (
	"context"

	"github.com/filanov/bm-inventory/models"
)

func (m *Manager) HostMonitoring() {
	var hosts []*models.Host

	monitorStates := []string{HostStatusDiscovering, HostStatusKnown, HostStatusDisconnected, HostStatusInsufficient, HostStatusPendingForInput}
	if err := m.db.Where("status IN (?)", monitorStates).Find(&hosts).Error; err != nil {
		m.log.WithError(err).Errorf("failed to get hosts")
		return
	}
	for _, host := range hosts {
		err := m.RefreshStatus(context.Background(), host, m.db)
		if err != nil {
			m.log.WithError(err).Errorf("failed to refresh host %s state", host.ID)
		}
	}
}

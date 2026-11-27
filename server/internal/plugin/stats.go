package plugin

import "sync/atomic"

// Stats 是插件可观测性快照；全部为进程内累计值，重启后归零。
type Stats struct {
	Registered  int
	Active      int
	Restarts    uint64
	APIRequests uint64
	APIErrors   uint64
}

type statsCounters struct {
	restarts    atomic.Uint64
	apiRequests atomic.Uint64
	apiErrors   atomic.Uint64
}

func (m *Manager) PluginStats() Stats {
	m.mu.Lock()
	registered := len(m.registrations)
	active := 0
	for _, inst := range m.instances {
		if inst.status == instanceRunning || inst.status == instanceBackoff {
			active++
		}
	}
	m.mu.Unlock()
	return Stats{
		Registered:  registered,
		Active:      active,
		Restarts:    m.stats.restarts.Load(),
		APIRequests: m.stats.apiRequests.Load(),
		APIErrors:   m.stats.apiErrors.Load(),
	}
}

func (m *Manager) recordRestart() { m.stats.restarts.Add(1) }

// APIStatsSink 供插件 API 服务上报调用量；避免 API 层依赖 Manager 具体类型。
type APIStatsSink interface {
	RecordAPI(success bool)
}

func (m *Manager) RecordAPI(success bool) {
	m.stats.apiRequests.Add(1)
	if !success {
		m.stats.apiErrors.Add(1)
	}
}

package main

import (
	"log/slog"
	"sort"
	"sync"

	"resurrector/util"
)

// Reconciler manages the set of Monitors and reconciles them against the desired
// state from config.toml. It implements the reconciliation algorithm described
// in doc/design.md.
type Reconciler struct {
	mu       sync.Mutex
	monitors map[string]*Monitor

	// onStateChange is forwarded to each Monitor for UI notifications.
	onStateChange func(MonitorStatus)
}

// NewReconciler creates a new Reconciler.
func NewReconciler(onStateChange func(MonitorStatus)) *Reconciler {
	return &Reconciler{
		monitors:      make(map[string]*Monitor),
		onStateChange: onStateChange,
	}
}

// Reconcile compares the desired state (from config.toml) against the current
// running state and performs the necessary actions to converge.
//
// Algorithm (from design.md):
//
//	for each entry in desired:
//	  if entry not in current              → START
//	  else if enabled changed to false      → KILL & STOP
//	  else if enabled changed to true       → START
//	  else if identity fields changed       → KILL → START
//	  else if only monitoring params changed → HOT-RELOAD
//	  else                                  → NO-OP
//
//	for each entry in current:
//	  if entry not in desired              → KILL & STOP (removed)
func (r *Reconciler) Reconcile(desired map[string]*util.App) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Process desired entries
	for name, desiredCfg := range desired {
		existing, exists := r.monitors[name]

		if !exists {
			// New entry — create monitor
			if desiredCfg.Enabled {
				slog.Info("reconcile: start monitor",
					slog.String("component", "reconciler"),
					slog.String("action", "START"),
					slog.String("app", name),
					slog.String("reason", "new entry"),
				)
				mon := NewMonitor(*desiredCfg, r.onStateChange)
				r.monitors[name] = mon
				mon.Start()
			} else {
				// New entry but disabled — just track it
				slog.Info("reconcile: track disabled entry",
					slog.String("component", "reconciler"),
					slog.String("action", "TRACK"),
					slog.String("app", name),
					slog.String("reason", "new entry, disabled"),
				)
				mon := NewMonitor(*desiredCfg, r.onStateChange)
				r.monitors[name] = mon
				// Emit Stopped state for the UI
				if r.onStateChange != nil {
					r.onStateChange(mon.Status())
				}
			}
			continue
		}

		currentCfg := existing.Config()

		// Check enabled state transitions
		if currentCfg.Enabled && !desiredCfg.Enabled {
			// Disabled — update config first so Stop()'s state notification reflects Enabled=false
			slog.Info("reconcile: stop monitor",
				slog.String("component", "reconciler"),
				slog.String("action", "STOP"),
				slog.String("app", name),
				slog.String("reason", "disabled"),
			)
			existing.SetConfig(*desiredCfg)
			existing.Stop()
			continue
		}

		if !currentCfg.Enabled && desiredCfg.Enabled {
			// Enabled — start monitoring
			slog.Info("reconcile: start monitor",
				slog.String("component", "reconciler"),
				slog.String("action", "START"),
				slog.String("app", name),
				slog.String("reason", "enabled"),
			)
			existing.Stop() // ensure stopped
			mon := NewMonitor(*desiredCfg, r.onStateChange)
			r.monitors[name] = mon
			mon.Start()
			continue
		}

		if !desiredCfg.Enabled {
			// Still disabled — update config reference but no action
			existing.UpdateMonitoringParams(*desiredCfg)
			continue
		}

		// Both enabled — check for identity changes
		if hasIdentityChanged(currentCfg, *desiredCfg) {
			slog.Info("reconcile: restart monitor",
				slog.String("component", "reconciler"),
				slog.String("action", "RESTART"),
				slog.String("app", name),
				slog.String("reason", "identity changed"),
			)
			existing.SetConfig(*desiredCfg)
			existing.Stop()
			mon := NewMonitor(*desiredCfg, r.onStateChange)
			r.monitors[name] = mon
			mon.Start()
			continue
		}

		// Check for monitoring parameter changes (hot-reload)
		if hasMonitoringParamsChanged(currentCfg, *desiredCfg) {
			slog.Info("reconcile: hot-reload monitor",
				slog.String("component", "reconciler"),
				slog.String("action", "HOT-RELOAD"),
				slog.String("app", name),
				slog.String("reason", "params changed"),
			)
			existing.UpdateMonitoringParams(*desiredCfg)
			continue
		}

		// No changes — NO-OP
	}

	// Process removed entries: entries in current but not in desired
	for name, mon := range r.monitors {
		if _, exists := desired[name]; !exists {
			slog.Info("reconcile: stop monitor",
				slog.String("component", "reconciler"),
				slog.String("action", "STOP"),
				slog.String("app", name),
				slog.String("reason", "removed from config"),
			)
			mon.Stop()
			// Notify UI to remove this entry
			if r.onStateChange != nil {
				status := mon.Status()
				status.State = StateRemoved
				r.onStateChange(status)
			}
			delete(r.monitors, name)
		}
	}
}

// StopAll stops all monitors. Called when the core process is exiting.
func (r *Reconciler) StopAll() {
	r.mu.Lock()
	monitors := make(map[string]*Monitor, len(r.monitors))
	for k, v := range r.monitors {
		monitors[k] = v
	}
	r.mu.Unlock()

	var wg sync.WaitGroup
	for name, mon := range monitors {
		wg.Add(1)
		go func(name string, mon *Monitor) {
			defer wg.Done()
			slog.Info("stopping monitor on shutdown",
				slog.String("component", "reconciler"),
				slog.String("app", name),
			)
			mon.Stop()
		}(name, mon)
	}
	wg.Wait()
	slog.Info("all monitors stopped",
		slog.String("component", "reconciler"),
	)
}

// AllStatuses returns a sorted list of all monitor statuses for the UI.
func (r *Reconciler) AllStatuses() []MonitorStatus {
	r.mu.Lock()
	defer r.mu.Unlock()

	statuses := make([]MonitorStatus, 0, len(r.monitors))
	for _, mon := range r.monitors {
		statuses = append(statuses, mon.Status())
	}
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Name < statuses[j].Name
	})
	return statuses
}

// hasIdentityChanged checks if any process-identity fields have changed,
// which requires a full process restart.
func hasIdentityChanged(current, desired util.App) bool {
	if current.Command != desired.Command {
		return true
	}
	if current.CWD != desired.CWD {
		return true
	}
	if current.HideWindow != desired.HideWindow {
		return true
	}
	if len(current.Args) != len(desired.Args) {
		return true
	}
	for i := range current.Args {
		if current.Args[i] != desired.Args[i] {
			return true
		}
	}
	return false
}

// hasMonitoringParamsChanged checks if only the monitoring parameters have
// changed. These can be hot-reloaded without restarting the process.
func hasMonitoringParamsChanged(current, desired util.App) bool {
	return current.RestartDelaySec != desired.RestartDelaySec ||
		current.HealthyTimeoutSec != desired.HealthyTimeoutSec ||
		current.MaxRetries != desired.MaxRetries ||
		current.StopTimeoutSec != desired.StopTimeoutSec ||
		!stringSlicesEqual(current.StopCommand, desired.StopCommand)
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

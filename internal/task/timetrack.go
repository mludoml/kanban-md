package task

import (
	"fmt"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/antopolskiy/kanban-md/internal/clierr"
	"github.com/antopolskiy/kanban-md/internal/config"
)

// Duration is a time.Duration stored as a Go duration string ("1h2m3s") in
// task frontmatter and JSON.
type Duration time.Duration

// MarshalYAML implements yaml.Marshaler.
func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	v, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	*d = Duration(v)
	return nil
}

// MarshalText implements encoding.TextMarshaler (used by JSON).
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler (used by JSON).
func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", b, err)
	}
	*d = Duration(v)
	return nil
}

// TrackStatusChange keeps the timer consistent with a status transition: the
// running timer is credited to oldStatus, and a new one starts when newStatus
// tracks time automatically and the task is not blocked.
func TrackStatusChange(t *Task, oldStatus, newStatus string, cfg *config.Config, now time.Time) {
	if oldStatus == newStatus {
		return
	}
	stopTimer(t, oldStatus, now)
	if cfg.StatusTimeTracking(newStatus) == config.TimeTrackingAuto && !t.Blocked {
		t.TimerStarted = &now
	}
}

// TrackBlockChange pauses the timer while a task is blocked and resumes it on
// unblock in an automatically tracked status.
func TrackBlockChange(t *Task, wasBlocked bool, cfg *config.Config, now time.Time) {
	switch {
	case !wasBlocked && t.Blocked:
		stopTimer(t, t.Status, now)
	case wasBlocked && !t.Blocked && t.TimerStarted == nil &&
		cfg.StatusTimeTracking(t.Status) == config.TimeTrackingAuto:
		t.TimerStarted = &now
	}
}

// StartTimer starts the timer in the task's current, time-tracked status.
// It returns false when the timer is already running.
func StartTimer(t *Task, cfg *config.Config, now time.Time) (bool, error) {
	if cfg.StatusTimeTracking(t.Status) == "" {
		return false, clierr.Newf(clierr.InvalidInput,
			"status %q does not track time (set time_tracking on it in config)", t.Status)
	}
	if t.Blocked {
		return false, clierr.Newf(clierr.StatusConflict, "task #%d is blocked", t.ID)
	}
	if t.TimerStarted != nil {
		return false, nil
	}
	t.TimerStarted = &now
	return true, nil
}

// StopTimer credits the running timer to the current status. It returns false
// when no timer was running.
func StopTimer(t *Task, now time.Time) bool {
	return stopTimer(t, t.Status, now)
}

func stopTimer(t *Task, status string, now time.Time) bool {
	if t.TimerStarted == nil {
		return false
	}
	if d := now.Sub(*t.TimerStarted).Round(time.Second); d > 0 {
		if t.TimeSpent == nil {
			t.TimeSpent = make(map[string]Duration)
		}
		t.TimeSpent[status] += Duration(d)
	}
	t.TimerStarted = nil
	return true
}

// TimeByStatus returns the tracked time per status at now, including the
// running timer (credited to the current status).
func TimeByStatus(t *Task, now time.Time) map[string]time.Duration {
	out := make(map[string]time.Duration, len(t.TimeSpent)+1)
	for s, d := range t.TimeSpent {
		out[s] += time.Duration(d)
	}
	if t.TimerStarted != nil {
		if d := now.Sub(*t.TimerStarted); d > 0 {
			out[t.Status] += d
		}
	}
	return out
}

// TimeTotal returns the total tracked time at now, including the running timer.
func TimeTotal(t *Task, now time.Time) time.Duration {
	var total time.Duration
	for _, d := range TimeByStatus(t, now) {
		total += d
	}
	return total
}

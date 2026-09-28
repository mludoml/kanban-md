package task_test

import (
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/antopolskiy/kanban-md/internal/config"
	"github.com/antopolskiy/kanban-md/internal/task"
)

func trackedConfig() *config.Config {
	cfg := config.NewDefault("Test")
	for i := range cfg.Statuses {
		switch cfg.Statuses[i].Name {
		case "in-progress":
			cfg.Statuses[i].TimeTracking = config.TimeTrackingAuto
		case "review":
			cfg.Statuses[i].TimeTracking = config.TimeTrackingManual
		}
	}
	return cfg
}

var t0 = time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)

func at(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }

func TestTimeTracking_AutoAndManualStatuses(t *testing.T) {
	cfg := trackedConfig()
	tk := &task.Task{ID: 1, Status: "in-progress"}

	task.TrackStatusChange(tk, "todo", "in-progress", cfg, at(0))
	tk.Status = "review"
	task.TrackStatusChange(tk, "in-progress", "review", cfg, at(30))
	if tk.TimerStarted != nil {
		t.Fatal("manual status must not start the timer on entry")
	}
	// Waiting in review does not count.
	if _, err := task.StartTimer(tk, cfg, at(90)); err != nil {
		t.Fatal(err)
	}
	if got := task.TimeTotal(tk, at(100)); got != 40*time.Minute {
		t.Fatalf("running total = %v, want 40m", got)
	}
	tk.Status = "done"
	task.TrackStatusChange(tk, "review", "done", cfg, at(105))

	byStatus := task.TimeByStatus(tk, at(500))
	if byStatus["in-progress"] != 30*time.Minute || byStatus["review"] != 15*time.Minute {
		t.Fatalf("by status = %v, want in-progress 30m, review 15m", byStatus)
	}
	if tk.TimerStarted != nil {
		t.Fatal("timer must stop on done")
	}
}

func TestTimeTracking_BlockPausesAutoTimer(t *testing.T) {
	cfg := trackedConfig()
	tk := &task.Task{ID: 1, Status: "in-progress"}
	task.TrackStatusChange(tk, "todo", "in-progress", cfg, at(0))

	tk.Blocked = true
	task.TrackBlockChange(tk, false, cfg, at(10))
	tk.Blocked = false
	task.TrackBlockChange(tk, true, cfg, at(50))

	if got := task.TimeTotal(tk, at(55)); got != 15*time.Minute {
		t.Fatalf("total = %v, want 15m (blocked time excluded)", got)
	}
}

func TestTimeTracking_StartRejectsUntrackedStatus(t *testing.T) {
	tk := &task.Task{ID: 1, Status: "todo"}
	if _, err := task.StartTimer(tk, trackedConfig(), at(0)); err == nil {
		t.Fatal("expected error starting a timer in an untracked status")
	}
}

func TestTimeTracking_FrontmatterRoundTrip(t *testing.T) {
	started := at(5)
	in := task.Task{
		ID: 1, Status: "review",
		TimeSpent:    map[string]task.Duration{"in-progress": task.Duration(time.Hour + 2*time.Minute)},
		TimerStarted: &started,
	}
	data, err := yaml.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out task.Task
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.TimeSpent["in-progress"] != in.TimeSpent["in-progress"] || !out.TimerStarted.Equal(started) {
		t.Fatalf("round trip lost time fields:\n%s", data)
	}
}

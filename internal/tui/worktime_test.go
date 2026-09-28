package tui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/antopolskiy/kanban-md/internal/config"
	"github.com/antopolskiy/kanban-md/internal/task"
	"github.com/antopolskiy/kanban-md/internal/tui"
)

// setupWorkTimeBoard: wave #1 (in-progress) with #2 done (1h in-progress +
// 10m review) and #3 in review (20m in-progress, review timer running 5m).
func setupWorkTimeBoard(t *testing.T, width, height int) *tui.Board {
	t.Helper()
	kanbanDir := filepath.Join(t.TempDir(), "kanban")
	tasksDir := filepath.Join(kanbanDir, "tasks")
	if err := os.MkdirAll(tasksDir, 0o750); err != nil {
		t.Fatal(err)
	}
	cfg := config.NewDefault("Work time")
	cfg.SetDir(kanbanDir)
	for i := range cfg.Statuses {
		switch cfg.Statuses[i].Name {
		case "in-progress":
			cfg.Statuses[i].TimeTracking = config.TimeTrackingAuto
		case "review":
			cfg.Statuses[i].TimeTracking = config.TimeTrackingManual
		case "done":
			show := true
			cfg.Statuses[i].ShowDuration = &show
		}
	}
	cfg.TUI.ParentLabel = "{title} (#{id})"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	reviewStart := now.Add(-5 * time.Minute)
	waveStart := now.Add(-3 * time.Hour)
	wave := 1
	tasks := []*task.Task{
		{ID: 1, Title: "Fala 5", Status: "in-progress", TimerStarted: &waveStart},
		{ID: 2, Title: "Done child", Status: "done", Parent: &wave, TimeSpent: map[string]task.Duration{
			"in-progress": task.Duration(time.Hour), "review": task.Duration(10 * time.Minute),
		}},
		{ID: 3, Title: "Review child", Status: "review", Parent: &wave, TimerStarted: &reviewStart,
			TimeSpent: map[string]task.Duration{"in-progress": task.Duration(20 * time.Minute)}},
	}
	for _, tk := range tasks {
		tk.Priority = "medium"
		tk.Updated = now
		if err := task.Write(filepath.Join(tasksDir, task.GenerateFilename(tk.ID, tk.Title)), tk); err != nil {
			t.Fatal(err)
		}
	}

	b := tui.NewBoard(cfg)
	b.SetNow(func() time.Time { return now })
	b.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return b
}

func TestWorkTime_ProjectStatsCountSubtasksOnly(t *testing.T) {
	b := setupWorkTimeBoard(t, 160, 30)
	v := ansi.Strip(b.View())
	// The wave's own 3h timer is ignored: 1h10m + 25m.
	want := "project time  total 1h35m  ·  in-progress 1h20m  ·  review 15m"
	if !strings.Contains(v, want) {
		t.Fatalf("missing %q in view:\n%s", want, v)
	}
	// Wave card shows the sum of its subtasks, running because #3 runs.
	if !strings.Contains(v, "▸1h35m") {
		t.Fatalf("wave card should show ▸1h35m:\n%s", v)
	}
	// Done card keeps its frozen time.
	if !strings.Contains(v, "1h10m") {
		t.Fatalf("done card should show 1h10m:\n%s", v)
	}
}

func TestWorkTime_GroupCardsKeepGeometry(t *testing.T) {
	for _, size := range [][2]int{{160, 30}, {60, 20}, {30, 12}} {
		b := setupWorkTimeBoard(t, size[0], size[1])
		v := b.View()
		lines := strings.Split(v, "\n")
		if len(lines) != size[1] {
			t.Fatalf("%dx%d: view has %d lines, want %d", size[0], size[1], len(lines), size[1])
		}
		for i, l := range lines {
			if w := lipgloss.Width(l); w > size[0] {
				t.Fatalf("%dx%d: line %d is %d wide: %q", size[0], size[1], i, w, ansi.Strip(l))
			}
		}
		if size[0] >= 160 && !strings.Contains(ansi.Strip(v), "─ Fala 5 (#1) ─") {
			t.Fatalf("%dx%d: missing group label:\n%s", size[0], size[1], ansi.Strip(v))
		}
	}
}

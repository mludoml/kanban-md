package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/antopolskiy/kanban-md/internal/task"
)

// timeStatsChrome is the framed project time line above the status bar,
// shown when any status tracks time: border, line, border.
const timeStatsChrome = 3

// runningMark prefixes a card's work time while a timer runs.
const runningMark = "▸"

// groupPalette colors parent groups; it avoids the default (240), active (62)
// and blocked (196) border colors.
var groupPalette = []string{"39", "208", "170", "42", "220", "141", "45", "180"}

func (b *Board) timeTracked() bool {
	return len(b.cfg.TrackedStatuses()) > 0
}

// workByStatus returns the tracked time per status of a task and whether a
// timer runs. A task with subtasks reports the sum of its subtasks instead of
// its own time, so a parent never counts twice.
func (b *Board) workByStatus(t *task.Task, now time.Time, seen map[int]bool) (map[string]time.Duration, bool) {
	kids := b.childrenOf[t.ID]
	if len(kids) == 0 || seen[t.ID] {
		return task.TimeByStatus(t, now), t.TimerStarted != nil
	}
	seen[t.ID] = true
	sum := make(map[string]time.Duration)
	running := false
	for _, k := range kids {
		byStatus, r := b.workByStatus(k, now, seen)
		for s, d := range byStatus {
			sum[s] += d
		}
		running = running || r
	}
	return sum, running
}

func (b *Board) workTime(t *task.Task) (time.Duration, map[string]time.Duration, bool) {
	byStatus, running := b.workByStatus(t, b.now(), map[int]bool{})
	var total time.Duration
	for _, d := range byStatus {
		total += d
	}
	return total, byStatus, running
}

// cardWorkTime renders a card's tracked work time, colored by age thresholds.
func (b *Board) cardWorkTime(t *task.Task) string {
	total, _, running := b.workTime(t)
	label := workDuration(total)
	if running {
		label = runningMark + label
	}
	return b.ageStyle(total).Render(label)
}

// detailWorkLines renders the tracked time in the detail view.
func (b *Board) detailWorkLines(t *task.Task) []string {
	if !b.timeTracked() {
		return nil
	}
	total, byStatus, running := b.workTime(t)
	if total == 0 && !running {
		return nil
	}
	line := workDuration(total) + b.statusBreakdown(byStatus, "(", ", ", ")")
	if running {
		line += "  " + runningMark + " timer running"
	}
	return []string{detailLabelStyle.Render("Work time:") + "  " + line}
}

// statusBreakdown lists tracked statuses in board order: "(in-progress 1h, review 5m)".
func (b *Board) statusBreakdown(byStatus map[string]time.Duration, open, sep, closing string) string {
	var parts []string
	for _, s := range b.cfg.TrackedStatuses() {
		parts = append(parts, s+" "+workDuration(byStatus[s]))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + open + strings.Join(parts, sep) + closing
}

// renderTimeStats renders the project time line: all tasks, archived
// included; parents are skipped since their time is their subtasks' time.
func (b *Board) renderTimeStats() string {
	now := b.now()
	byStatus := make(map[string]time.Duration)
	var total time.Duration
	for _, t := range b.allTasks {
		if len(b.childrenOf[t.ID]) > 0 {
			continue
		}
		for s, d := range task.TimeByStatus(t, now) {
			byStatus[s] += d
			total += d
		}
	}
	inner := b.width - 4 //nolint:mnd // border + padding on both sides
	var sb strings.Builder
	sb.WriteString(timeStatsLabelStyle.Render("project time"))
	sb.WriteString(statusBarStyle.Render("  total "))
	sb.WriteString(timeStatsValueStyle.Render(workDuration(total)))
	for _, s := range b.cfg.TrackedStatuses() {
		sb.WriteString(statusBarStyle.Render("  ·  " + s + " "))
		sb.WriteString(timeStatsValueStyle.Render(workDuration(byStatus[s])))
	}
	line := sb.String()
	if lipgloss.Width(line) > inner {
		plain := fmt.Sprintf("project time  total %s%s", workDuration(total), b.statusBreakdown(byStatus, " ·  ", "  ·  ", ""))
		line = statusBarStyle.Render(truncate(plain, inner))
	}
	return timeStatsBoxStyle.Width(max(b.width-2, 0)).Render(line) //nolint:mnd // border width
}

var (
	timeStatsLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)
	timeStatsValueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	timeStatsBoxStyle   = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("238")).
				Padding(0, 1)
)

// workDuration formats tracked time: "<1m", "45m", "2h05m", "31h40m".
func workDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	default:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60) //nolint:mnd // minutes per hour
	}
}

// cardGroup returns the parent ID whose group a card belongs to (its own ID
// for a parent), or 0 when parent grouping is off or the task has no group.
func (b *Board) cardGroup(t *task.Task) int {
	if b.cfg.TUI.ParentLabel == "" {
		return 0
	}
	if t.Parent != nil {
		return *t.Parent
	}
	if len(b.childrenOf[t.ID]) > 0 {
		return t.ID
	}
	return 0
}

// renderGroupCard renders a card with a border in its group's color and the
// group label in the top border. The selected card gets a thick border.
func (b *Board) renderGroupCard(t *task.Task, group int, active bool, content string, width int) string {
	color := lipgloss.Color(groupPalette[group%len(groupPalette)])
	border := lipgloss.RoundedBorder()
	if active {
		border = lipgloss.ThickBorder()
	}
	borderColor := color
	if t.Blocked {
		borderColor = lipgloss.Color("196")
	}
	body := lipgloss.NewStyle().
		Border(border).
		BorderTop(false).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(width - 2). //nolint:mnd // border width
		Render(content)
	label := b.groupLabel(group)
	return groupTopBorder(border, label, width, borderColor, color) + "\n" + body
}

// groupLabel expands tui.parent_label for a parent: {id} and {title}.
func (b *Board) groupLabel(parentID int) string {
	title := ""
	for _, t := range b.allTasks {
		if t.ID == parentID {
			title = t.Title
			break
		}
	}
	return strings.NewReplacer("{id}", strconv.Itoa(parentID), "{title}", title).Replace(b.cfg.TUI.ParentLabel)
}

// groupTopBorder draws "╭─ label ────╮" exactly width cells wide.
func groupTopBorder(border lipgloss.Border, label string, width int, borderColor, labelColor lipgloss.TerminalColor) string {
	edge := lipgloss.NewStyle().Foreground(borderColor)
	inner := width - 2 //nolint:mnd // corners
	if inner < 1 {
		return edge.Render(border.TopLeft + border.TopRight)
	}
	const lead = 1 // one border cell before the label
	text := ""
	if inner > lead+2 { //nolint:mnd // room for " x "
		text = truncate(" "+label+" ", inner-lead)
	}
	fill := inner - lead - lipgloss.Width(text)
	return edge.Render(border.TopLeft+strings.Repeat(border.Top, lead)) +
		lipgloss.NewStyle().Foreground(labelColor).Bold(true).Render(text) +
		edge.Render(strings.Repeat(border.Top, fill)+border.TopRight)
}

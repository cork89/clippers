// ./internal/views/processing_helpers.go
package views

type StageState int

const (
	StagePending StageState = iota
	StageRunning
	StageComplete
	StageFailed
)

func stageIcon(state StageState) string {
	switch state {
	case StageComplete:
		return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>`
	case StageRunning:
		return `<svg class="spinning" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"/></svg>`
	case StageFailed:
		return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>`
	default:
		return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/></svg>`
	}
}

func stageClass(state StageState) string {
	switch state {
	case StageComplete:
		return "complete"
	case StageRunning:
		return "running"
	case StageFailed:
		return "failed"
	default:
		return "pending"
	}
}

func stageClassForStage(stageIndex, currentStage int) string {
	if currentStage > stageIndex {
		return "complete"
	} else if currentStage == stageIndex {
		return "running"
	}
	return "pending"
}

func stageIconForStage(stageIndex, currentStage int) string {
	if currentStage > stageIndex {
		return stageIcon(StageComplete)
	} else if currentStage == stageIndex {
		return stageIcon(StageRunning)
	}
	return stageIcon(StagePending)
}

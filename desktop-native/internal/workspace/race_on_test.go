//go:build race

package workspace

// raceEnabled: the race detector allocates on its own, so budgets do not apply.
const raceEnabled = true

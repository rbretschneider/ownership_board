package domain

// RunnerDirectory maps worker ids to their assigned names.
type RunnerDirectory map[string]RunnerName

// NewRunnerDirectory indexes assigned names by worker id.
func NewRunnerDirectory(assigned []RunnerName) RunnerDirectory {
	directory := make(RunnerDirectory, len(assigned))
	for _, name := range assigned {
		if name.IsAssigned() {
			directory[name.Worker] = name
		}
	}
	return directory
}

// Lookup returns the name assigned to a worker identity, if any.
func (d RunnerDirectory) Lookup(identity string) (RunnerName, bool) {
	workerID, isWorker := WorkerIDOf(identity)
	if !isWorker {
		return RunnerName{}, false
	}
	name, isNamed := d[workerID]
	return name, isNamed
}

// Label is RunnerLabel with the directory's name for the identity.
func (d RunnerDirectory) Label(identity string) string {
	name, _ := d.Lookup(identity)
	return RunnerLabel(identity, name.Name)
}

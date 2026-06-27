package maintenance

import "errors"

var ErrUnknownRunType = errors.New("unknown maintenance run type")

type RunExecutor func(Run) error

func DispatchRun(run Run, executors map[string]RunExecutor) error {
	executor := executors[run.RunType]
	if executor == nil {
		return ErrUnknownRunType
	}
	return executor(run)
}

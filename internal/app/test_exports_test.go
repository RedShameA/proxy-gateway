package app

type MaintenanceRunnerForTest struct {
	r *maintenanceRunner
}

func (g *Gateway) MaintenanceForTest() MaintenanceRunnerForTest {
	return MaintenanceRunnerForTest{r: g.maintenance}
}

func (r MaintenanceRunnerForTest) RunQueuedTasksForTest() {
	if r.r != nil {
		r.r.runQueuedTasks()
	}
}

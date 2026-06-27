package proxy

const (
	RequestLogStateRunning   = "running"
	RequestLogStateCompleted = "completed"
)

const (
	RequestLogResultRunning = "running"
	RequestLogResultSuccess = "success"
	RequestLogResultFailure = "failure"
)

const (
	RequestLogEventKindStart   = "start"
	RequestLogEventKindFinish  = "finish"
	RequestLogEventKindFailure = "failure"
	RequestLogEventKindUnknown = "unknown"
)

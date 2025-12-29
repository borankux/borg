package executor

// Job represents a job to execute
type Job struct {
	TaskID           string
	JobID            string
	JobName          string
	Type             string // shell, binary, docker, executor_binary
	Command          string
	Args             []string
	Env              map[string]string
	WorkingDirectory string
	TimeoutSeconds   int64
	DockerImage      string
	Privileged       bool
	RequiredFiles    []string
	ExecutorBinaryID string
	TaskData         map[string]interface{} // CSV row data
}


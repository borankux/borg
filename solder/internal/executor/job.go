package executor

// Job represents a job to execute
type Job struct {
	TaskID           string
	JobID            string
	JobName          string
	Type             string // shell, binary, docker
	Command          string
	Args             []string
	Env              map[string]string
	WorkingDirectory string
	TimeoutSeconds   int64
	DockerImage      string
	Privileged       bool
	RequiredFiles    []string
}


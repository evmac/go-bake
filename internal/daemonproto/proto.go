package daemonproto

// Request is a single command sent from the bake CLI to baked.
type Request struct {
	Run       string `json:"run,omitempty"`       // target or suite name to run
	Up        bool   `json:"up,omitempty"`        // run workflow up
	Down      bool   `json:"down,omitempty"`      // run down
	Daemon    string `json:"daemon,omitempty"`    // for down: optional daemon name
	Reload    bool   `json:"reload,omitempty"`    // force config reload
	Status    bool   `json:"status,omitempty"`    // request daemon status dump
	Workspace string `json:"workspace,omitempty"` // workspace root (for multi-workspace)
}

// Response is the result of executing a request.
type Response struct {
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
	Output   string `json:"output,omitempty"` // optional payload (e.g. status dump)
}

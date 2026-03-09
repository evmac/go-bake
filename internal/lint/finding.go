package lint

// Finding is one linter result (file:line: message).
type Finding struct {
	File    string
	Line    int
	Column  int
	Message string
	RuleID  string
	Fixable bool
	// TargetName and StepIndex identify the location for AST-level fixes (e.g. prefer-exec).
	TargetName string
	StepIndex  int
}

package workflow

// Definition represents a workflow template
type Definition struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Steps       []*Step `json:"steps"`
}

// GetPredefinedWorkflows returns all available workflow definitions
func GetPredefinedWorkflows() map[string]*Definition {
	return map[string]*Definition{
		"feature_implementation": FeatureImplementation(),
		"bug_fix_verification":   BugFixWithVerification(),
		"code_review":            CodeReview(),
		"migration":              Migration(),
		"refactor":               Refactor(),
	}
}

// FeatureImplementation creates a workflow for implementing new features
func FeatureImplementation() *Definition {
	return &Definition{
		Name:        "feature_implementation",
		Description: "Structured workflow for implementing new features with analysis, planning, execution, and verification",
		Steps: []*Step{
			{
				ID:          "analyze",
				Name:        "Analyze Requirements",
				Type:        StepTypeAnalyze,
				Description: "Analyze the codebase to understand dependencies and identify files that need modification",
				Status:      StepStatusPending,
			},
			{
				ID:          "plan",
				Name:        "Create Implementation Plan",
				Type:        StepTypePlan,
				Description: "Generate a detailed implementation plan with file changes and approach",
				Status:      StepStatusPending,
				Requires:    []string{"analyze"},
			},
			{
				ID:          "approve_plan",
				Name:        "Approve Plan",
				Type:        StepTypeApprove,
				Description: "Wait for user approval of the implementation plan",
				Status:      StepStatusPending,
				Requires:    []string{"plan"},
			},
			{
				ID:          "execute",
				Name:        "Implement Feature",
				Type:        StepTypeExecute,
				Description: "Apply the planned changes to implement the feature",
				Status:      StepStatusPending,
				Requires:    []string{"approve_plan"},
			},
			{
				ID:          "verify",
				Name:        "Verify Implementation",
				Type:        StepTypeVerify,
				Description: "Run tests and validate the implementation works correctly",
				Status:      StepStatusPending,
				Requires:    []string{"execute"},
			},
		},
	}
}

// BugFixWithVerification creates a workflow for fixing bugs with verification
func BugFixWithVerification() *Definition {
	return &Definition{
		Name:        "bug_fix_verification",
		Description: "Workflow for fixing bugs with root cause analysis and verification",
		Steps: []*Step{
			{
				ID:          "reproduce",
				Name:        "Reproduce Bug",
				Type:        StepTypeAnalyze,
				Description: "Identify and reproduce the bug to understand its behavior",
				Status:      StepStatusPending,
			},
			{
				ID:          "analyze",
				Name:        "Analyze Root Cause",
				Type:        StepTypeAnalyze,
				Description: "Analyze the code to identify the root cause of the bug",
				Status:      StepStatusPending,
				Requires:    []string{"reproduce"},
			},
			{
				ID:          "plan",
				Name:        "Create Fix Plan",
				Type:        StepTypePlan,
				Description: "Plan the fix with minimal changes to resolve the issue",
				Status:      StepStatusPending,
				Requires:    []string{"analyze"},
			},
			{
				ID:          "approve_fix",
				Name:        "Approve Fix",
				Type:        StepTypeApprove,
				Description: "Wait for user approval of the fix approach",
				Status:      StepStatusPending,
				Requires:    []string{"plan"},
			},
			{
				ID:          "execute",
				Name:        "Apply Fix",
				Type:        StepTypeExecute,
				Description: "Apply the bug fix to the codebase",
				Status:      StepStatusPending,
				Requires:    []string{"approve_fix"},
				RollbackCmd: "git checkout -- .",
			},
			{
				ID:          "verify",
				Name:        "Verify Fix",
				Type:        StepTypeVerify,
				Description: "Run tests to verify the fix resolves the issue without introducing regressions",
				Status:      StepStatusPending,
				Requires:    []string{"execute"},
			},
		},
	}
}

// CodeReview creates a workflow for reviewing code changes
func CodeReview() *Definition {
	return &Definition{
		Name:        "code_review",
		Description: "Workflow for systematic code review with checklist verification",
		Steps: []*Step{
			{
				ID:          "identify_changes",
				Name:        "Identify Changes",
				Type:        StepTypeAnalyze,
				Description: "Identify all files and changes to be reviewed",
				Status:      StepStatusPending,
			},
			{
				ID:          "review_logic",
				Name:        "Review Logic",
				Type:        StepTypeAnalyze,
				Description: "Review the logic and correctness of the changes",
				Status:      StepStatusPending,
				Requires:    []string{"identify_changes"},
			},
			{
				ID:          "review_style",
				Name:        "Review Style",
				Type:        StepTypeAnalyze,
				Description: "Review code style and best practices",
				Status:      StepStatusPending,
				Requires:    []string{"review_logic"},
			},
			{
				ID:          "review_security",
				Name:        "Review Security",
				Type:        StepTypeAnalyze,
				Description: "Review for potential security issues",
				Status:      StepStatusPending,
				Requires:    []string{"review_style"},
			},
			{
				ID:          "summarize",
				Name:        "Summarize Findings",
				Type:        StepTypePlan,
				Description: "Create a summary of review findings and recommendations",
				Status:      StepStatusPending,
				Requires:    []string{"review_security"},
			},
		},
	}
}

// Migration creates a workflow for code migrations
func Migration() *Definition {
	return &Definition{
		Name:        "migration",
		Description: "Workflow for migrating code from one pattern/framework to another",
		Steps: []*Step{
			{
				ID:          "analyze",
				Name:        "Analyze Current State",
				Type:        StepTypeAnalyze,
				Description: "Analyze the current codebase to identify all code needing migration",
				Status:      StepStatusPending,
			},
			{
				ID:          "plan",
				Name:        "Create Migration Plan",
				Type:        StepTypePlan,
				Description: "Create a step-by-step migration plan with file ordering",
				Status:      StepStatusPending,
				Requires:    []string{"analyze"},
			},
			{
				ID:          "approve",
				Name:        "Approve Migration",
				Type:        StepTypeApprove,
				Description: "Wait for user approval of migration plan",
				Status:      StepStatusPending,
				Requires:    []string{"plan"},
			},
			{
				ID:          "execute",
				Name:        "Execute Migration",
				Type:        StepTypeExecute,
				Description: "Apply migration changes in the planned order",
				Status:      StepStatusPending,
				Requires:    []string{"approve"},
			},
			{
				ID:          "verify",
				Name:        "Verify Migration",
				Type:        StepTypeVerify,
				Description: "Run all tests and verify the migration is complete",
				Status:      StepStatusPending,
				Requires:    []string{"execute"},
			},
			{
				ID:          "cleanup",
				Name:        "Cleanup",
				Type:        StepTypeExecute,
				Description: "Remove deprecated code and clean up",
				Status:      StepStatusPending,
				Requires:    []string{"verify"},
			},
		},
	}
}

// Refactor creates a workflow for code refactoring
func Refactor() *Definition {
	return &Definition{
		Name:        "refactor",
		Description: "Workflow for safe code refactoring with tests",
		Steps: []*Step{
			{
				ID:          "analyze",
				Name:        "Analyze Code",
				Type:        StepTypeAnalyze,
				Description: "Analyze the code to identify refactoring opportunities",
				Status:      StepStatusPending,
			},
			{
				ID:          "ensure_tests",
				Name:        "Ensure Test Coverage",
				Type:        StepTypeVerify,
				Description: "Verify existing test coverage or create tests for safety",
				Status:      StepStatusPending,
				Requires:    []string{"analyze"},
			},
			{
				ID:          "plan",
				Name:        "Create Refactor Plan",
				Type:        StepTypePlan,
				Description: "Plan the refactoring steps",
				Status:      StepStatusPending,
				Requires:    []string{"ensure_tests"},
			},
			{
				ID:          "approve",
				Name:        "Approve Refactor",
				Type:        StepTypeApprove,
				Description: "Wait for user approval",
				Status:      StepStatusPending,
				Requires:    []string{"plan"},
			},
			{
				ID:          "execute",
				Name:        "Execute Refactor",
				Type:        StepTypeExecute,
				Description: "Apply refactoring changes",
				Status:      StepStatusPending,
				Requires:    []string{"approve"},
			},
			{
				ID:          "verify",
				Name:        "Verify Refactor",
				Type:        StepTypeVerify,
				Description: "Run tests to verify refactoring didn't break anything",
				Status:      StepStatusPending,
				Requires:    []string{"execute"},
			},
		},
	}
}

// GetDefinition retrieves a workflow definition by name
func GetDefinition(name string) (*Definition, bool) {
	workflows := GetPredefinedWorkflows()
	def, ok := workflows[name]
	return def, ok
}

// ListDefinitions returns the names of all available workflow definitions
func ListDefinitions() []string {
	workflows := GetPredefinedWorkflows()
	names := make([]string, 0, len(workflows))
	for name := range workflows {
		names = append(names, name)
	}
	return names
}

package ai

import (
	"fmt"
	"regexp"
	"strings"
)

const CodeAssistantSystemPrompt = `
You are floyd, an AI assistant with expertise in programming and software development.
Your task is to assist with coding-related questions, debugging, refactoring, and explaining code.

Guidelines:
- Provide clear, concise, and accurate responses
- Include code examples where helpful
- Prioritize modern best practices
- If you're unsure, acknowledge limitations instead of guessing
- Focus on understanding the user's intent, even if the question is ambiguous
`

const CodeGenerationSystemPrompt = `
You are floyd, an AI assistant focused on helping write high-quality code.
Your task is to generate code based on user requirements and specifications.

Guidelines:
- Write clean, efficient, and well-documented code
- Follow language-specific best practices and conventions
- Include helpful comments explaining complex sections
- Prioritize maintainability and readability
- Structure code logically with appropriate error handling
- Consider edge cases and potential issues
`

const CodeReviewSystemPrompt = `
You are floyd, an AI code reviewer with expertise in programming best practices.
Your task is to analyze code, identify issues, and suggest improvements.

Guidelines:
- Look for bugs, security issues, and performance problems
- Suggest improvements for readability and maintainability
- Identify potential edge cases and error handling gaps
- Point out violations of best practices or conventions
- Provide constructive feedback with clear explanations
- Be thorough but prioritize important issues over minor stylistic concerns
`

const CodeExplanationSystemPrompt = `
You are floyd, an AI assistant that specializes in explaining code.
Your task is to break down and explain code in a clear, educational manner.

Guidelines:
- Explain the purpose and functionality of the code
- Break down complex parts step by step
- Define technical terms and concepts when relevant
- Use analogies or examples to illustrate concepts
- Focus on the core logic rather than trivial details
- Adjust explanation depth based on the apparent complexity of the question
`

type PromptTemplate struct {
	Template string
	System   string
	Defaults map[string]string
}

var PromptTemplates = map[string]PromptTemplate{
	"explainCode": {
		Template: "Please explain what this code does:\n\n{code}",
		System:   CodeExplanationSystemPrompt,
		Defaults: map[string]string{
			"code": "// Paste code here",
		},
	},
	"refactorCode": {
		Template: "Please refactor this code to improve its {focus}:\n\n{code}\n\nAdditional context: {context}",
		System:   CodeGenerationSystemPrompt,
		Defaults: map[string]string{
			"focus":   "readability and maintainability",
			"code":    "// Paste code here",
			"context": "None",
		},
	},
	"debugCode": {
		Template: "Please help me debug the following code:\n\n{code}\n\nThe issue I'm seeing is: {issue}\n\nAny error messages: {errorMessages}",
		System:   CodeAssistantSystemPrompt,
		Defaults: map[string]string{
			"code":          "// Paste code here",
			"issue":         "Describe the issue you're experiencing",
			"errorMessages": "None",
		},
	},
	"reviewCode": {
		Template: "Please review this code and provide feedback:\n\n{code}",
		System:   CodeReviewSystemPrompt,
		Defaults: map[string]string{
			"code": "// Paste code here",
		},
	},
	"generateCode": {
		Template: "Please write code to {task}.\n\nLanguage/Framework: {language}\n\nRequirements:\n{requirements}",
		System:   CodeGenerationSystemPrompt,
		Defaults: map[string]string{
			"task":         "Describe what you want the code to do",
			"language":     "Specify language or framework",
			"requirements": "- List your requirements here",
		},
	},
	"documentCode": {
		Template: "Please add documentation to this code:\n\n{code}\n\nDocumentation style: {style}",
		System:   CodeGenerationSystemPrompt,
		Defaults: map[string]string{
			"code":  "// Paste code here",
			"style": "Standard comments and docstrings",
		},
	},
	"testCode": {
		Template: "Please write tests for this code:\n\n{code}\n\nTesting framework: {framework}",
		System:   CodeGenerationSystemPrompt,
		Defaults: map[string]string{
			"code":      "// Paste code here",
			"framework": "Specify testing framework or 'standard'",
		},
	},
}

func FormatPrompt(template string, values map[string]any, defaults map[string]string) string {
	merged := map[string]string{}
	for k, v := range defaults {
		merged[k] = v
	}
	for k, v := range values {
		merged[k] = fmt.Sprint(v)
	}

	re := regexp.MustCompile(`\{(\w+)\}`)
	return re.ReplaceAllStringFunc(template, func(match string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(match, "{"), "}")
		if value, ok := merged[key]; ok {
			return value
		}
		return match
	})
}

func UsePromptTemplate(templateName string, values map[string]any) (string, string, error) {
	template, ok := PromptTemplates[templateName]
	if !ok {
		return "", "", fmt.Errorf("prompt template %q not found", templateName)
	}

	prompt := FormatPrompt(template.Template, values, template.Defaults)
	return prompt, template.System, nil
}

func CreateConversation(prompt, system string) []Message {
	messages := []Message{}
	if system != "" {
		messages = append(messages, Message{Role: RoleSystem, Content: system})
	}
	messages = append(messages, Message{Role: RoleUser, Content: prompt})
	return messages
}

func CreateUserMessage(content string) Message {
	return Message{Role: RoleUser, Content: content}
}

func CreateSystemMessage(content string) Message {
	return Message{Role: RoleSystem, Content: content}
}

func CreateAssistantMessage(content string) Message {
	return Message{Role: RoleAssistant, Content: content}
}

func CreateFileContextMessage(filePath, content, language string) string {
	lang := language
	if lang == "" {
		lang = languageFromPath(filePath)
	}
	return fmt.Sprintf("File: %s\n\n```%s\n%s\n```", filePath, lang, content)
}

func languageFromPath(filePath string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepathExt(filePath), "."))
	languageMap := map[string]string{
		"js":      "javascript",
		"ts":      "typescript",
		"jsx":     "javascript",
		"tsx":     "typescript",
		"py":      "python",
		"rb":      "ruby",
		"java":    "java",
		"c":       "c",
		"cpp":     "cpp",
		"cs":      "csharp",
		"go":      "go",
		"rs":      "rust",
		"php":     "php",
		"swift":   "swift",
		"kt":      "kotlin",
		"scala":   "scala",
		"sh":      "bash",
		"html":    "html",
		"css":     "css",
		"scss":    "scss",
		"sass":    "sass",
		"less":    "less",
		"md":      "markdown",
		"json":    "json",
		"yml":     "yaml",
		"yaml":    "yaml",
		"toml":    "toml",
		"sql":     "sql",
		"graphql": "graphql",
		"xml":     "xml",
	}
	if lang, ok := languageMap[ext]; ok {
		return lang
	}
	return ""
}

func filepathExt(path string) string {
	idx := strings.LastIndex(path, ".")
	if idx == -1 {
		return ""
	}
	return path[idx:]
}

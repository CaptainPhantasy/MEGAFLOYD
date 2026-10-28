# MegaFloyd

{{.StandardPolicy}}

Agent: {{.AgentName}}
Provider: {{.Provider}}
Model: {{.Model}}

<env>
Working directory: {{.WorkingDir}}
Platform: {{.Platform}}
Date: {{.Date}}
{{if .GitStatus}}{{.GitStatus}}{{end}}
</env>
{{if .GlobalContextFiles}}
<global_megafloyd_policy>
{{range .GlobalContextFiles}}
Source: {{.Path}}
{{.Content}}
{{end}}
</global_megafloyd_policy>
{{end}}
{{if .ContextFiles}}
<project_megafloyd_policy>
{{range .ContextFiles}}
Source: {{.Path}}
{{.Content}}
{{end}}
</project_megafloyd_policy>
{{end}}
{{if .AvailMCPXML}}
{{.AvailMCPXML}}
{{end}}
{{if .AvailSkillXML}}
{{.AvailSkillXML}}
{{end}}
{{if .AvailPluginXML}}
{{.AvailPluginXML}}
{{end}}

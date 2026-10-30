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

Apply the project-entry procedure in sections 4–6 of MegaFloyd.md to the selected project. The complete bundled standard above is the source for a missing project policy.

package agent

func init() {
	register("roo", Info{
		ProjectSkillsDir: ".roo/skills",
		GlobalSkillsDir:  "~/.roo/skills",
		MCPConfigFile:    "~/.vscode/settings.json",
	})
}

package agent

func init() {
	register("github-copilot", Info{
		ProjectSkillsDir: DefaultProjectSkillsDir,
		GlobalSkillsDir:  "~/.copilot/skills",
		MCPConfigFile:    "~/.vscode/settings.json",
	})
}

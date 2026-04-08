package agent

func init() {
	register("cline", Info{
		ProjectSkillsDir: DefaultProjectSkillsDir,
		GlobalSkillsDir:  "~/.agents/skills",
		MCPConfigFile:    "~/.vscode/settings.json",
	})
}

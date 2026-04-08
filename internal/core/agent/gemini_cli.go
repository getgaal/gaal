package agent

func init() {
	register("gemini-cli", Info{
		ProjectSkillsDir: DefaultProjectSkillsDir,
		GlobalSkillsDir:  "~/.gemini/skills",
		MCPConfigFile:    "~/.gemini/settings.json",
	})
}

package agent

func init() {
	register("openhands", Info{
		ProjectSkillsDir: ".openhands/skills",
		GlobalSkillsDir:  "~/.openhands/skills",
		MCPConfigFile:    "~/.openhands/config.json",
	})
}

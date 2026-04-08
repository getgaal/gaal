package agent

func init() {
	register("antigravity", Info{
		ProjectSkillsDir: DefaultProjectSkillsDir,
		GlobalSkillsDir:  "~/.gemini/antigravity/skills",
		MCPConfigFile:    "",
	})
}

package agent

func init() {
	register("goose", Info{
		ProjectSkillsDir: ".goose/skills",
		GlobalSkillsDir:  "~/.config/goose/skills",
		MCPConfigFile:    "~/.config/goose/config.yaml",
	})
}

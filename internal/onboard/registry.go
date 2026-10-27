package onboard

// Register the built-in tools via their package init functions. The core needs
// the registry for BuiltinSkillNames/BuiltinToolNames, so every consumer of this
// package (CLI and control-plane) gets a populated registry. The CLI also
// imports these packages in cmd/myaaw/main.go; a duplicate blank import is a
// no-op because init runs once per process.
import (
	_ "myaaw/internal/agent/tools/a2aauth"
	_ "myaaw/internal/agent/tools/a2ainput"
	_ "myaaw/internal/agent/tools/bash"
	_ "myaaw/internal/agent/tools/contact"
	_ "myaaw/internal/agent/tools/cron"
	_ "myaaw/internal/agent/tools/filesystem"
	_ "myaaw/internal/agent/tools/provider"
	_ "myaaw/internal/agent/tools/python"
	_ "myaaw/internal/agent/tools/subagent"
)

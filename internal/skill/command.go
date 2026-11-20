package skill

const skillName = "foco"

func installerArgs(source string, options Options) []string {
	args := []string{
		"--yes",
		"--ignore-scripts",
		"skills@1.7.0",
		"add",
		source,
		"--skill",
		skillName,
		"--copy",
	}
	for _, agent := range options.Agents {
		args = append(args, "--agent", agent)
	}
	if options.Global {
		args = append(args, "--global")
	}
	if options.Yes {
		args = append(args, "--yes")
	}
	return args
}

func commandFor(goos, source string, options Options, comspec string) (string, []string) {
	args := installerArgs(source, options)
	if goos == "windows" {
		if comspec == "" {
			comspec = "cmd.exe"
		}
		return comspec, windowsInstallerCommand(args)
	}
	return "npx", args
}

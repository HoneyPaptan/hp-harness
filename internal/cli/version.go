package cli

func cmdVersion(env Env, args []string) int {
	if len(args) != 0 {
		return exitUsage(env, usagef("unexpected arguments"))
	}
	printVersion(env)
	return 0
}

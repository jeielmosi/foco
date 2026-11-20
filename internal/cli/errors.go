package cli

type CliError struct{ msg string }

func (e *CliError) Error() string { return e.msg }

func usageError(msg string) error { return &CliError{msg: msg} }

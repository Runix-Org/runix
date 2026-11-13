//go:build !devtools

package cli

import "runix/internal/ui/product"

type debugOptions struct{}

func (*Command) hasDebugOption() bool {
	return false
}

func (*Command) applyDebugOptions(*product.Options, bool) error {
	return nil
}

func (*Command) appendDebugRestartArgs(args []string) []string {
	return args
}

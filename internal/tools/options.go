package tools

import "github.com/cloudwego/eino/components/tool"

// toolOptions holds custom options for knowhow tools.
type toolOptions struct {
	VaultID string
}

// WithVaultID returns a tool.Option that sets the vault ID for tool execution.
func WithVaultID(vaultID string) tool.Option {
	return tool.WrapImplSpecificOptFn(func(o *toolOptions) {
		o.VaultID = vaultID
	})
}

// getToolOptions extracts knowhow-specific options from eino tool options.
func getToolOptions(opts ...tool.Option) *toolOptions {
	return tool.GetImplSpecificOptions(&toolOptions{}, opts...)
}

package mcp

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	mcppkg "github.com/opendatahub-io/odh-cli/pkg/mcp"
	utilcobra "github.com/opendatahub-io/odh-cli/pkg/util/cobra"
)

const (
	cmdName  = "mcp"
	cmdShort = "Model Context Protocol server"

	serveName  = "serve"
	serveShort = "Start MCP server exposing CLI tools"
	serveLong  = `Start a Model Context Protocol (MCP) server that exposes all odh CLI
commands as typed tool calls with structured inputs and outputs.

This lets any MCP-capable agent (Claude, GPT, Cursor, etc.) call odh
operations programmatically. The server inherits kubeconfig from the
process environment.

Transports:
  stdio  JSON-RPC over stdin/stdout (default, works in containers and CI)
  sse    HTTP Server-Sent Events for network-accessible mode

Examples:
  # Start MCP server on stdio (default)
  kubectl odh mcp serve

  # Start MCP server with SSE transport
  kubectl odh mcp serve --transport sse --port 8080
`
)

const defaultPort = 8080

// AddCommand adds the mcp command group to the root command.
func AddCommand(root *cobra.Command, flags *genericclioptions.ConfigFlags, opts ...utilcobra.CommandOption) {
	mcpCmd := &cobra.Command{
		Use:   cmdName,
		Short: cmdShort,
	}

	var (
		transport string
		port      int
	)

	serveCmd := &cobra.Command{
		Use:           serveName,
		Short:         serveShort,
		Long:          serveLong,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			srv := mcppkg.NewServer(flags, mcppkg.Transport(transport), port)

			return srv.Serve(cmd.Context())
		},
	}

	serveCmd.Flags().StringVar(&transport, "transport", "stdio", `Transport protocol: "stdio" or "sse"`)
	serveCmd.Flags().IntVar(&port, "port", defaultPort, "Port for SSE transport")

	mcpCmd.AddCommand(serveCmd)

	utilcobra.ApplyOptions(mcpCmd, opts...)

	root.AddCommand(mcpCmd)
}

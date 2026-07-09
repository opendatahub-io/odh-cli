package mcp

import (
	"bytes"
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	pkgcmd "github.com/opendatahub-io/odh-cli/pkg/cmd"
)

// toolDefinition pairs an MCP tool schema with a handler.
type toolDefinition struct {
	tool    mcp.Tool
	handler server.ToolHandlerFunc
}

// commandFactory creates a fresh Command instance for each MCP tool call.
type commandFactory func(
	streams genericiooptions.IOStreams,
	configFlags *genericclioptions.ConfigFlags,
) pkgcmd.Command

// argumentApplier sets MCP request arguments onto a Command before execution.
type argumentApplier func(command pkgcmd.Command, request mcp.CallToolRequest) error

// toolAdapter bridges a Command to an MCP tool handler.
type toolAdapter struct {
	configFlags *genericclioptions.ConfigFlags
	factory     commandFactory
	applier     argumentApplier
}

func (a *toolAdapter) handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var outBuf, errBuf bytes.Buffer

	streams := genericiooptions.IOStreams{
		In:     &bytes.Buffer{},
		Out:    &outBuf,
		ErrOut: &errBuf,
	}

	flags := cloneConfigFlags(a.configFlags)
	command := a.factory(streams, flags)

	if err := a.applier(command, request); err != nil {
		return mapErrorToResult(err), nil
	}

	if err := command.Complete(); err != nil {
		return mapErrorToResult(err), nil
	}

	if err := command.Validate(); err != nil {
		return mapErrorToResult(err), nil
	}

	if err := command.Run(ctx); err != nil {
		return mapErrorToResult(err), nil
	}

	output := outBuf.String()
	if output == "" {
		output = "{}"
	}

	return mcp.NewToolResultText(output), nil
}

func cloneConfigFlags(src *genericclioptions.ConfigFlags) *genericclioptions.ConfigFlags {
	dst := genericclioptions.NewConfigFlags(true)

	dst.CacheDir = src.CacheDir
	dst.KubeConfig = src.KubeConfig
	dst.ClusterName = src.ClusterName
	dst.AuthInfoName = src.AuthInfoName
	dst.Context = src.Context
	dst.Namespace = src.Namespace
	dst.APIServer = src.APIServer
	dst.TLSServerName = src.TLSServerName
	dst.Insecure = src.Insecure
	dst.CertFile = src.CertFile
	dst.KeyFile = src.KeyFile
	dst.CAFile = src.CAFile
	dst.BearerToken = src.BearerToken
	dst.Impersonate = src.Impersonate
	dst.ImpersonateUID = src.ImpersonateUID
	dst.ImpersonateGroup = src.ImpersonateGroup
	dst.ImpersonateUserExtra = src.ImpersonateUserExtra
	dst.Username = src.Username
	dst.Password = src.Password
	dst.Timeout = src.Timeout
	dst.DisableCompression = src.DisableCompression
	dst.WrapConfigFn = src.WrapConfigFn

	return dst
}

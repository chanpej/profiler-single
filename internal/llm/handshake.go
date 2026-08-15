package llm

import (
	"context"
	"fmt"
	"io"

	acp "github.com/coder/acp-go-sdk"
)

func acpHandshake(ctx context.Context, client acp.Client, stdin io.Writer, stdout io.Reader, cwd string) (*acp.ClientSideConnection, acp.NewSessionResponse, error) {
	conn := acp.NewClientSideConnection(client, stdin, stdout)
	initResp, err := conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientInfo:      &acp.Implementation{Name: "UnityPerfAgent", Version: "1.0.0"},
		ClientCapabilities: acp.ClientCapabilities{
			Meta: map[string]any{"parameterizedModelPicker": true},
		},
	})
	if err != nil {
		return nil, acp.NewSessionResponse{}, fmt.Errorf("llm: acp initialize: %w", err)
	}
	if id := pickAuthMethodID(initResp.AuthMethods); id != "" {
		if _, err := conn.Authenticate(ctx, acp.AuthenticateRequest{MethodId: id}); err != nil {
			return nil, acp.NewSessionResponse{}, fmt.Errorf("llm: acp authenticate (%s): %w", id, err)
		}
	}
	sess, err := conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        cwd,
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		return nil, acp.NewSessionResponse{}, fmt.Errorf("llm: acp session/new: %w", err)
	}
	return conn, sess, nil
}

func pickAuthMethodID(methods []acp.AuthMethod) string {
	ids := make([]string, 0, len(methods))
	for _, m := range methods {
		if id := authMethodID(m); id != "" {
			ids = append(ids, id)
		}
	}
	return preferAuthID(ids)
}

func preferAuthID(ids []string) string {
	for _, id := range ids {
		if id == "cursor_login" {
			return id
		}
	}
	if len(ids) > 0 {
		return ids[0]
	}
	return ""
}

func authMethodID(m acp.AuthMethod) string {
	switch {
	case m.Agent != nil:
		return m.Agent.Id
	case m.EnvVar != nil:
		return m.EnvVar.Id
	case m.Terminal != nil:
		return m.Terminal.Id
	default:
		return ""
	}
}

package agent

import "github.com/sipeed/picoclaw/pkg/logger"

// storeContextOnly armazena uma mensagem na sessão sem chamar o LLM.
// Usado pelo modo "listen all" do Discord (ouvir tudo, responder só quando mencionado).
func (al *AgentLoop) storeContextOnly(agent *AgentInstance, sessionKey, content string) (string, error) {
	agent.Sessions.AddMessage(sessionKey, "user", "[context] "+content)
	agent.Sessions.Save(sessionKey)

	logger.DebugCF("agent", "Stored context-only message",
		map[string]interface{}{
			"agent_id":    agent.ID,
			"session_key": sessionKey,
			"content_len": len(content),
		})

	return "", nil
}

package office

// ToolAvailability is verified execution capability, not the saved preferences
// of an agent. Providers are eligible only in the already selected run backend.
type ToolAvailability struct {
	Ready           bool
	Backend         string
	ProviderFamily  string
	Capabilities    map[string]bool
	ModelSelection  bool
	EffortSelection bool
}

type AttemptEvidence struct {
	State             string // never_started, settled, running, unknown
	Failure           string // unavailable, rate_limited, authentication, provider_failed, transport
	EffectsReconciled bool
}

type FallbackRequest struct {
	Agent                      Agent
	Backend                    string
	Previous                   AttemptEvidence
	Used                       []ExecutionChoice
	Available                  map[string]ToolAvailability
	RequiredCapabilities       []string
	ForbiddenProviderFamilies  []string
	RequireKnownProviderFamily bool
	CoordinatorAcknowledged    bool
	HumanApproved              bool
	OfficePaused               bool
}

type FallbackDecision struct {
	Status  string            `json:"status"`
	Choice  *ExecutionChoice  `json:"choice,omitempty"`
	Reason  string            `json:"reason"`
	Skipped map[string]string `json:"skipped"`
}

// SelectFallback cannot start work. It evaluates whether a separately fenced
// attempt may be proposed, approved or dispatched; it never treats a timeout as
// proof of exit and never changes the agent's saved principal or appearance.
func SelectFallback(in FallbackRequest) FallbackDecision {
	out := FallbackDecision{Status: "blocked", Skipped: map[string]string{}}
	deny := func(status, reason string) FallbackDecision { out.Status = status; out.Reason = reason; return out }
	if in.OfficePaused {
		return deny("blocked", "O escritório está pausado; fallback não pode contornar a parada.")
	}
	if in.Agent.Status != "active" {
		return deny("blocked", "O integrante não está ativo.")
	}
	if in.Backend != "orca" && in.Backend != "native" {
		return deny("blocked", "Backend do run não comprovado.")
	}
	switch in.Previous.Failure {
	case "unavailable", "rate_limited", "authentication", "provider_failed", "transport":
	default:
		return deny("blocked", "Este resultado não autoriza trocar de ferramenta.")
	}
	switch in.Previous.State {
	case "never_started":
	case "settled":
		if !in.Previous.EffectsReconciled {
			return deny("unknown", "Concilie os efeitos da tentativa encerrada antes de liberar outra.")
		}
	case "running", "unknown":
		return deny("unknown", "A tentativa anterior ainda pode executar; nenhuma alternativa será iniciada.")
	default:
		return deny("unknown", "O estado da tentativa anterior não foi comprovado.")
	}
	used := map[ExecutionChoice]bool{in.Agent.Primary: true}
	for _, choice := range in.Used {
		used[choice] = true
	}
	allowed := map[string]bool{}
	for _, capability := range in.Agent.AllowedCapabilities {
		allowed[capability] = true
	}
	for _, required := range in.RequiredCapabilities {
		if !allowed[required] {
			return deny("blocked", "O trabalho exige ferramenta fora do escopo salvo do integrante.")
		}
	}
	for _, choice := range in.Agent.Fallbacks {
		key := choice.ToolID + ":" + choice.Model + ":" + choice.Effort
		if used[choice] {
			out.Skipped[key] = "Tentativa já utilizada neste episódio."
			continue
		}
		tool, ok := in.Available[choice.ToolID]
		if !ok || !tool.Ready {
			out.Skipped[key] = "Ferramenta sem disponibilidade comprovada."
			continue
		}
		if tool.Backend != in.Backend {
			out.Skipped[key] = "Alternativa não está disponível no backend fixado do run."
			continue
		}
		if (choice.Model != "" && !tool.ModelSelection) || (choice.Effort != "" && !tool.EffortSelection) {
			out.Skipped[key] = "Modelo ou esforço não suportado pela combinação verificada."
			continue
		}
		if in.RequireKnownProviderFamily && tool.ProviderFamily == "" {
			out.Skipped[key] = "Família do provider não comprovada para revisão independente."
			continue
		}
		forbidden := false
		for _, family := range in.ForbiddenProviderFamilies {
			if family == tool.ProviderFamily {
				forbidden = true
				break
			}
		}
		if forbidden {
			out.Skipped[key] = "Alternativa viola a independência exigida da revisão."
			continue
		}
		capable := true
		for _, required := range in.RequiredCapabilities {
			if !tool.Capabilities[required] {
				capable = false
				break
			}
		}
		if !capable {
			out.Skipped[key] = "Alternativa não comprova as capacidades necessárias."
			continue
		}
		selected := choice
		out.Choice = &selected
		if !in.CoordinatorAcknowledged {
			return deny("awaiting_coordination", "O coordenador precisa registrar ciência da nova tentativa.")
		}
		automaticSafe := in.Agent.FallbackMode == "automatic_safe" && in.Previous.State == "never_started" && (in.Previous.Failure == "unavailable" || in.Previous.Failure == "rate_limited")
		if !automaticSafe && !in.HumanApproved {
			return deny("needs_approval", "Confirme a alternativa e o escopo antes de uma nova tentativa.")
		}
		return deny("ready", "Alternativa elegível; o gateway ainda deve revalidar política, escopo e autorização antes do dispatch.")
	}
	return deny("exhausted", "Nenhuma alternativa elegível permanece neste episódio; não haverá ciclo de retries.")
}

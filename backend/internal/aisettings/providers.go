package aisettings

// Provider is a fixed, reviewed destination. Clients cannot supply URLs or headers.
type Provider struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Models   []string `json:"models"`
	Endpoint string   `json:"endpoint"`
}

func Providers() []Provider {
	return []Provider{
		{ID: "openai", Name: "OpenAI", Models: []string{"gpt-4.1-mini", "gpt-4.1"}, Endpoint: "https://api.openai.com/v1/responses"},
		{ID: "aiwanwu", Name: "aiwanwu (third-party relay)", Models: []string{"gpt-6-sol"}, Endpoint: "https://2api.aiwanwu.cc/v1/responses"},
	}
}

func ResolveProvider(provider, model string) (Provider, bool) {
	for _, p := range Providers() {
		if p.ID == provider {
			for _, m := range p.Models {
				if m == model {
					return p, true
				}
			}
		}
	}
	return Provider{}, false
}

func ValidModel(provider, model string) bool {
	_, ok := ResolveProvider(provider, model)
	return ok
}

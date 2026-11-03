package sdk

import "github.com/xiaokuang95/nulo/sdk/internal/ai"

func CreateAgentSession(opt CreateAgentSessionServicesOptions, start SessionStartOptions) (*Runtime, *Session, error) {
	svcs, err := CreateAgentSessionServices(opt)
	if err != nil {
		return nil, nil, err
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		return nil, nil, err
	}
	sess, err := rt.New(start)
	if err != nil {
		_ = rt.Close()
		return nil, nil, err
	}
	return rt, sess, nil
}

func BuildModel(spec string, svcs *AgentSessionServices) (Model, error) {
	if spec == "" && svcs != nil && svcs.SettingsManager != nil {
		provider, modelID := svcs.SettingsManager.GetDefaultProvider(), svcs.SettingsManager.GetDefaultModel()
		if modelID != "" {
			spec = modelID
			if provider != "" {
				spec = provider + "/" + modelID
			}
		}
	}
	switch spec {
	case "", "faux", "faux/faux-1":
		return DefaultFauxModel(), nil
	}
	dir := ""
	var providers ai.Providers
	if svcs != nil {
		dir = svcs.AgentDir
		if svcs.plugins != nil || len(svcs.dynamicProviders) > 0 {
			providers = svcs.providerSnapshot()
		}
	}
	if providers == nil {
		var err error
		providers, err = ai.LoadProviders(dir)
		if err != nil {
			return Model{}, err
		}
	}
	return providers.FindModel(spec)
}

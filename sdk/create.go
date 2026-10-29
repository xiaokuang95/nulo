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
	switch spec {
	case "", "faux", "faux/faux-1":
		return DefaultFauxModel(), nil
	}
	dir := ""
	if svcs != nil {
		dir = svcs.AgentDir
	}
	models := ai.LoadModels(dir)
	if m, ok := ai.FindModel(spec, models); ok {
		return m, nil
	}
	return Model{}, errUnknownModel(spec)
}

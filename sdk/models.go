package sdk

import (
	"fmt"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func DefaultFauxModel() Model { return ai.DefaultFauxModel() }

func errUnknownModel(spec string) error {
	return fmt.Errorf("unknown model %q", spec)
}

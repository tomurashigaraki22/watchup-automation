package logging

import (
	"go.uber.org/zap"
)

// New builds a Zap logger appropriate for the environment.
// Production yields structured JSON; development yields human-friendly console.
func New(appEnv string) (*zap.Logger, error) {
	if appEnv == "production" {
		return zap.NewProduction()
	}
	return zap.NewDevelopment()
}

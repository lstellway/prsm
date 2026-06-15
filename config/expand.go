package config

import "os"

// expandSecrets applies os.ExpandEnv to auth.token and auth.password on every
// provider. Only these two fields are expanded; all other config values are
// used as-is to avoid accidentally expanding user-supplied strings such as
// repo names or view descriptions that may contain "$".
func expandSecrets(cfg *Config) {
	for i := range cfg.Providers {
		cfg.Providers[i].Auth.Token = os.ExpandEnv(cfg.Providers[i].Auth.Token)
		cfg.Providers[i].Auth.Password = os.ExpandEnv(cfg.Providers[i].Auth.Password)
	}
}

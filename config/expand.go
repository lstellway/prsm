package config

import "os"

// expandSecrets applies os.ExpandEnv to auth.token and auth.password on every
// provider. Only these two fields are expanded; all other config values are
// used as-is to avoid accidentally expanding user-supplied strings such as
// repo names or view descriptions that may contain "$".
func expandSecrets(loadedConfig *Config) {
	for index := range loadedConfig.Providers {
		loadedConfig.Providers[index].Auth.Token = os.ExpandEnv(loadedConfig.Providers[index].Auth.Token)
		loadedConfig.Providers[index].Auth.Password = os.ExpandEnv(loadedConfig.Providers[index].Auth.Password)
	}
}

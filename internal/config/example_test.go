package config

import (
	"fmt"
	"os"
	"path/filepath"
)

func ExampleLoadWithOptions() {
	dir, err := os.MkdirTemp("", "dns-update-config-example-*")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer os.RemoveAll(dir)

	tokenFile := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenFile, []byte("secret"), 0o600); err != nil {
		fmt.Println(err)
		return
	}

	configPath := filepath.Join(dir, "config.json")
	configJSON := `{
  "record": {
    "name": "host.example.com.",
    "zone": "example.com.",
    "ttl_seconds": 300
  },
  "provider": {
    "type": "cloudflare",
    "cloudflare": {
      "zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
      "api_token_file": "` + tokenFile + `"
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		fmt.Println(err)
		return
	}

	cfg, err := LoadWithOptions(LoadOptions{
		Path:         configPath,
		ExplicitPath: true,
	})
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(cfg.Record.Name)
	fmt.Println(cfg.Record.TTLSeconds)
	fmt.Println(cfg.Probe.IPv4URL.String())
	// Output:
	// host.example.com.
	// 300
	// https://4.ip.omsab.net/
}

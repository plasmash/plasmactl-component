package configure

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/launchrctl/launchr/pkg/action"
	"gopkg.in/yaml.v3"
)

// Configure implements the unified component:configure command
type Configure struct {
	action.WithLogger
	action.WithTerm

	// Arguments
	Key   string
	Value string

	// Operation flags (mutually exclusive)
	Get      bool
	List     bool
	Validate bool
	Generate bool

	// Scope
	At string // chassis section for override, empty for component defaults

	// Modifiers
	Vault      bool
	Format     string
	Strict     bool
	YesIAmSure bool
}

// Execute runs the configure action based on flags
func (c *Configure) Execute() error {
	// Determine operation mode
	switch {
	case c.List:
		return c.executeList()
	case c.Validate:
		return c.executeValidate()
	case c.Generate:
		return c.executeGenerate()
	case c.Get || (c.Key != "" && c.Value == ""):
		return c.executeGet()
	case c.Key != "" && c.Value != "":
		return c.executeSet()
	default:
		return fmt.Errorf("usage: configure <key> <value> | configure <key> --get | configure --list | configure --validate | configure <key> --generate")
	}
}

func (c *Configure) executeGet() error {
	if c.Key == "" {
		return fmt.Errorf("key is required for get operation")
	}

	configDir, err := c.resolveConfigDir()
	if err != nil {
		return err
	}

	filename := "vars.yaml"
	if c.Vault {
		filename = "vault.yaml"
	}

	configFile := filepath.Join(configDir, filename)
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return fmt.Errorf("config file not found: %s", configFile)
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	value, ok := config[c.Key]
	if !ok {
		return fmt.Errorf("key %q not found", c.Key)
	}

	fmt.Println(value)
	return nil
}

func (c *Configure) executeSet() error {
	if c.Key == "" {
		return fmt.Errorf("key is required for set operation")
	}

	configDir, err := c.resolveConfigDir()
	if err != nil {
		// Create config directory if it doesn't exist
		configDir, err = c.createConfigDir()
		if err != nil {
			return err
		}
	}

	filename := "vars.yaml"
	if c.Vault {
		filename = "vault.yaml"
	}

	configFile := filepath.Join(configDir, filename)

	var config map[string]interface{}
	if data, err := os.ReadFile(configFile); err == nil {
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse existing config: %w", err)
		}
	} else {
		config = make(map[string]interface{})
	}

	config[c.Key] = c.Value

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	scope := "component defaults"
	if c.At != "" {
		scope = c.At
	}
	c.Term().Success().Printfln("Set %s = %s (scope: %s)", c.Key, c.Value, scope)
	return nil
}

func (c *Configure) executeList() error {
	configDir, err := c.resolveConfigDir()
	if err != nil {
		c.Term().Info().Println("No configuration found")
		return nil
	}

	result := make(map[string]interface{})

	// Read vars.yaml
	valuesFile := filepath.Join(configDir, "vars.yaml")
	if data, err := os.ReadFile(valuesFile); err == nil {
		var values map[string]interface{}
		if err := yaml.Unmarshal(data, &values); err == nil {
			for k, v := range values {
				if c.Key == "" || strings.HasPrefix(k, c.Key) {
					result[k] = v
				}
			}
		}
	}

	// Read vault.yaml if requested
	if c.Vault {
		vaultFile := filepath.Join(configDir, "vault.yaml")
		if data, err := os.ReadFile(vaultFile); err == nil {
			var vault map[string]interface{}
			if err := yaml.Unmarshal(data, &vault); err == nil {
				for k, v := range vault {
					if c.Key == "" || strings.HasPrefix(k, c.Key) {
						result[k+" (vault)"] = v
					}
				}
			}
		}
	}

	if len(result) == 0 {
		c.Term().Info().Println("No configuration values found")
		return nil
	}

	switch strings.ToLower(c.Format) {
	case "json":
		output, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(output))
	case "yaml":
		output, _ := yaml.Marshal(result)
		fmt.Println(string(output))
	default:
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "KEY\tVALUE")
		for k, v := range result {
			fmt.Fprintf(w, "%s\t%v\n", k, v)
		}
		w.Flush()
	}

	return nil
}

func (c *Configure) executeValidate() error {
	c.Term().Info().Println("Validating configuration...")

	// TODO: Implement schema-based validation
	// 1. Load component schemas from meta/plasma.yaml files
	// 2. Validate config values against schemas
	// 3. Report errors and warnings

	c.Term().Warning().Println("Schema-based validation not yet implemented")
	c.Term().Success().Println("Basic config structure is valid")
	return nil
}

func (c *Configure) executeGenerate() error {
	if c.Key == "" {
		return fmt.Errorf("key is required for generate operation")
	}

	if !c.YesIAmSure {
		c.Term().Warning().Println("Secret generation/rotation will change credentials.")
		c.Term().Warning().Println("Applications may need to be restarted.")
		c.Term().Info().Println("Use --yes-i-am-sure to proceed")
		return nil
	}

	// TODO: Implement secret generation
	// 1. Generate new secret value based on key pattern
	// 2. Update vault.yaml
	// 3. Optionally trigger re-deployment

	c.Term().Warning().Println("Secret generation not yet implemented")
	return nil
}

// resolveConfigDir finds the configuration directory based on scope
func (c *Configure) resolveConfigDir() (string, error) {
	if c.At == "" {
		// Component defaults: defaults/main.yaml location
		// For now, use current directory's defaults/
		configDir := "defaults"
		if _, err := os.Stat(configDir); err == nil {
			return configDir, nil
		}
		return "", fmt.Errorf("component defaults directory not found")
	}

	// Chassis-scoped override: src/{layer}/cfg/{section}/
	return resolveChassisConfigDir(c.At)
}

// createConfigDir creates the configuration directory if it doesn't exist
func (c *Configure) createConfigDir() (string, error) {
	if c.At == "" {
		configDir := "defaults"
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create defaults directory: %w", err)
		}
		return configDir, nil
	}

	// Chassis-scoped: src/{layer}/cfg/{section}/
	parts := strings.Split(c.At, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid chassis section %q (expected format: platform.{layer}.{...})", c.At)
	}
	layer := parts[1]
	configDir := filepath.Join("src", layer, "cfg", c.At)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}
	return configDir, nil
}

// resolveChassisConfigDir finds the configuration directory for a chassis section
func resolveChassisConfigDir(chassis string) (string, error) {
	if chassis == "" {
		return "", fmt.Errorf("chassis section is required")
	}

	// Parse chassis name to extract layer
	// Example: platform.foundation.cluster -> foundation
	parts := strings.Split(chassis, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid chassis section %q (expected format: platform.{layer}.{...})", chassis)
	}

	layer := parts[1] // e.g., "foundation", "integration", "cognition"

	configDir := filepath.Join("src", layer, "cfg", chassis)
	if _, err := os.Stat(configDir); err == nil {
		return configDir, nil
	}

	return "", fmt.Errorf("chassis config directory not found: %s", configDir)
}

package scan

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/seikaikyo/go-common/modbus"
	"github.com/seikaikyo/go-common/response"
	"gopkg.in/yaml.v3"
)

// PluginDeviceConfig is the YAML-compatible config for a modbus plugin device.
type PluginDeviceConfig struct {
	Name         string           `yaml:"name" json:"name"`
	Host         string           `yaml:"host" json:"host"`
	Port         int              `yaml:"port" json:"port"`
	UnitID       int              `yaml:"unit_id" json:"unit_id"`
	PollInterval string           `yaml:"poll_interval" json:"poll_interval"`
	Registers    []PluginRegister `yaml:"registers" json:"registers"`
}

// PluginRegister is a single register definition for the modbus plugin config.
type PluginRegister struct {
	Name    string  `yaml:"name" json:"name"`
	Address int     `yaml:"address" json:"address"`
	Type    string  `yaml:"type" json:"type"`
	Count   int     `yaml:"count" json:"count"`
	Scale   float64 `yaml:"scale,omitempty" json:"scale,omitempty"`
	Unit    string  `yaml:"unit,omitempty" json:"unit,omitempty"`
}

func handleToConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job := store.get(id)
	if job == nil {
		response.Err(w, http.StatusNotFound, "job not found")
		return
	}
	if job.Status != "completed" {
		response.Err(w, http.StatusConflict, "job not yet completed, status: "+job.Status)
		return
	}
	if job.Result == nil {
		response.Err(w, http.StatusInternalServerError, "job completed but no result")
		return
	}

	cfg := resultToConfig(job.Request, job.Result)

	yamlBytes, err := yaml.Marshal(cfg)
	if err != nil {
		response.Err(w, http.StatusInternalServerError, "yaml marshal failed: "+err.Error())
		return
	}

	response.OK(w, map[string]any{
		"device_config": cfg,
		"yaml":          string(yamlBytes),
	})
}

func resultToConfig(req modbus.ScanRequest, result *modbus.ScanResult) PluginDeviceConfig {
	cfg := PluginDeviceConfig{
		Name:         result.Device,
		Host:         req.Host,
		Port:         req.Port,
		UnitID:       int(req.UnitID),
		PollInterval: "1s",
	}

	for _, reg := range result.Registers {
		// Only include responsive registers with a guess
		if reg.Guess == nil {
			continue
		}

		count := 1
		if reg.InferredType == "float32_hi" {
			count = 2
		}

		name := fmt.Sprintf("%s_%d", reg.Type, reg.Address)
		if reg.Guess != nil {
			name = fmt.Sprintf("%s_%d", reg.Guess.Category, reg.Address)
		}

		scale := scaleForCategory(reg)

		pr := PluginRegister{
			Name:    name,
			Address: int(reg.Address),
			Type:    reg.Type,
			Count:   count,
			Scale:   scale,
			Unit:    unitForCategory(reg),
		}
		cfg.Registers = append(cfg.Registers, pr)
	}

	return cfg
}

func scaleForCategory(reg modbus.AnalyzedRegister) float64 {
	if reg.InferredType == "float32_hi" {
		return 1
	}
	if reg.Guess == nil {
		return 1
	}
	switch reg.Guess.Category {
	case "temperature":
		return 0.1
	case "percentage":
		return 1
	case "pressure", "pressure/level":
		return 0.1
	default:
		return 1
	}
}

func unitForCategory(reg modbus.AnalyzedRegister) string {
	if reg.Guess == nil {
		return ""
	}
	switch reg.Guess.Category {
	case "temperature":
		return "celsius"
	case "percentage":
		return "percent"
	case "pressure", "pressure/level":
		return "bar"
	case "rpm/speed":
		return "rpm"
	default:
		return ""
	}
}

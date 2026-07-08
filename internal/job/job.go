// Package job loads and validates the render Job payload.
package job

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/lascade/motwr/internal/config"
)

type Vehicle string

const (
	VehicleCar   Vehicle = "car"
	VehicleBoat  Vehicle = "boat"
	VehiclePlane Vehicle = "plane"
	VehicleTrain Vehicle = "train"
)

// Job is the minimal render payload (see spec: minimal job JSON schema).
type Job struct {
	Title    string  `json:"title"`
	Subtitle string  `json:"subtitle"`
	Script   string  `json:"script"`
	Vehicle  Vehicle `json:"vehicle"`
}

func (j *Job) validate() error {
	switch {
	case strings.TrimSpace(j.Title) == "":
		return fmt.Errorf("job: title is required")
	case strings.TrimSpace(j.Subtitle) == "":
		return fmt.Errorf("job: subtitle is required")
	case strings.TrimSpace(j.Script) == "":
		return fmt.Errorf("job: script is required")
	}
	switch j.Vehicle {
	case VehicleCar, VehicleBoat, VehiclePlane, VehicleTrain:
		return nil
	default:
		return fmt.Errorf("job: vehicle must be one of car|boat|plane|train, got %q", j.Vehicle)
	}
}

// Parse unmarshals and validates a job payload.
func Parse(data []byte) (*Job, error) {
	var j Job
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("job: invalid JSON: %w", err)
	}
	if err := j.validate(); err != nil {
		return nil, err
	}
	return &j, nil
}

// Load reads the job from a local path or an http(s) URL.
func Load(pathOrURL string) (*Job, error) {
	var data []byte
	if strings.HasPrefix(pathOrURL, "http://") || strings.HasPrefix(pathOrURL, "https://") {
		client := &http.Client{Timeout: config.JobFetchTimeout}
		resp, err := client.Get(pathOrURL)
		if err != nil {
			return nil, fmt.Errorf("job: fetch %s: %w", pathOrURL, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("job: fetch %s: HTTP %d", pathOrURL, resp.StatusCode)
		}
		data, err = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return nil, fmt.Errorf("job: read %s: %w", pathOrURL, err)
		}
	} else {
		var err error
		data, err = os.ReadFile(pathOrURL)
		if err != nil {
			return nil, fmt.Errorf("job: %w", err)
		}
	}
	return Parse(data)
}

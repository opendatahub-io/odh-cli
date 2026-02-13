package yaml_test

import (
	"bytes"
	"testing"

	k8syaml "sigs.k8s.io/yaml"

	"github.com/opendatahub-io/odh-cli/pkg/printer/yaml"
)

type testStruct struct {
	Name   string `yaml:"name"`
	Age    int    `yaml:"age"`
	Active bool   `yaml:"active"`
}

func TestRenderer_Render(t *testing.T) {
	tests := []struct {
		name    string
		value   testStruct
		wantErr bool
	}{
		{
			name: "simple struct",
			value: testStruct{
				Name:   "Alice",
				Age:    30,
				Active: true,
			},
			wantErr: false,
		},
		{
			name: "empty struct",
			value: testStruct{
				Name:   "",
				Age:    0,
				Active: false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			renderer := yaml.NewRenderer[testStruct](
				yaml.WithWriter[testStruct](&buf),
			)

			err := renderer.Render(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)

				return
			}

			if !tt.wantErr {
				// Verify output is valid YAML
				var result testStruct
				if err := k8syaml.Unmarshal(buf.Bytes(), &result); err != nil {
					t.Errorf("Output is not valid YAML: %v", err)

					return
				}

				// Verify values match
				if result.Name != tt.value.Name || result.Age != tt.value.Age || result.Active != tt.value.Active {
					t.Errorf("Render() output mismatch: got %+v, want %+v", result, tt.value)
				}
			}
		})
	}
}

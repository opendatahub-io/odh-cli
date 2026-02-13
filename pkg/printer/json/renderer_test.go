package json_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	printerjson "github.com/opendatahub-io/odh-cli/pkg/printer/json"
)

type testStruct struct {
	Name   string `json:"name"`
	Age    int    `json:"age"`
	Active bool   `json:"active"`
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
			renderer := printerjson.NewRenderer[testStruct](
				printerjson.WithWriter[testStruct](&buf),
			)

			err := renderer.Render(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)

				return
			}

			if !tt.wantErr {
				// Verify output is valid JSON
				var result testStruct
				if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
					t.Errorf("Output is not valid JSON: %v", err)

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

func TestRenderer_WithIndent(t *testing.T) {
	var buf bytes.Buffer
	value := testStruct{Name: "Bob", Age: 25, Active: true}

	renderer := printerjson.NewRenderer[testStruct](
		printerjson.WithWriter[testStruct](&buf),
		printerjson.WithIndent[testStruct]("    "),
	)
	if err := renderer.Render(value); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Verify output contains indentation (4 spaces)
	output := buf.String()
	if !strings.Contains(output, "    ") {
		t.Error("Output does not contain expected indentation")
	}
}

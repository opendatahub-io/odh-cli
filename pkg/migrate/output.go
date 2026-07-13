package migrate

import (
	"encoding/json"
	"fmt"
	"io"

	"sigs.k8s.io/yaml"

	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
)

func actionIOForMode(streams iostreams.Interface, structured bool) iostreams.Interface {
	if structured {
		return iostreams.NewFullQuietWrapper(streams)
	}

	return streams
}

func writeStructuredOutput(w io.Writer, format OutputFormat, result any) error {
	switch format { //nolint:exhaustive // table is handled by caller before this function
	case OutputFormatJSON:
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}

		if _, err = fmt.Fprintln(w, string(data)); err != nil {
			return fmt.Errorf("writing JSON output: %w", err)
		}

		return nil
	case OutputFormatYAML:
		data, err := yaml.Marshal(result)
		if err != nil {
			return fmt.Errorf("marshaling YAML: %w", err)
		}

		if _, err = fmt.Fprint(w, string(data)); err != nil {
			return fmt.Errorf("writing YAML output: %w", err)
		}

		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}
}

package confirmation

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/lburgazzoli/odh-cli/pkg/util/iostreams"
)

func Prompt(io iostreams.Interface, message string) bool {
	_, _ = fmt.Fprintf(io.ErrOut(), "%s [y/N]: ", message)

	reader := bufio.NewReader(io.In())
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))

	return response == "y" || response == "yes"
}

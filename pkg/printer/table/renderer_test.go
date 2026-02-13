package table_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/printer/table"

	. "github.com/onsi/gomega"
)

type testPerson struct {
	Name   string
	Age    int
	Status string
}

type testPersonWithTags struct {
	Name     string
	Tags     []string
	Metadata map[string]any
}

func TestRendererWithSliceInput(t *testing.T) {
	g := NewWithT(t)

	var buf bytes.Buffer
	renderer := table.NewRenderer[[]any](
		table.WithWriter[[]any](&buf),
		table.WithHeaders[[]any]("Name", "Age"),
	)

	err := renderer.Append([]any{"Alice", 30})
	g.Expect(err).ShouldNot(HaveOccurred())

	err = renderer.Render()
	g.Expect(err).ShouldNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).Should(ContainSubstring("Alice"))
	g.Expect(output).Should(ContainSubstring("30"))
}

func TestRendererWithStructInput(t *testing.T) {
	g := NewWithT(t)

	var buf bytes.Buffer
	renderer := table.NewRenderer[testPerson](
		table.WithWriter[testPerson](&buf),
		table.WithHeaders[testPerson]("Name", "Age", "Status"),
	)

	person := testPerson{
		Name:   "Alice",
		Age:    30,
		Status: "active",
	}

	err := renderer.Append(person)
	g.Expect(err).ShouldNot(HaveOccurred())

	err = renderer.Render()
	g.Expect(err).ShouldNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).Should(ContainSubstring("Alice"))
	g.Expect(output).Should(ContainSubstring("30"))
	g.Expect(output).Should(ContainSubstring("active"))
}

func TestRendererWithCustomFormatter(t *testing.T) {
	g := NewWithT(t)

	var buf bytes.Buffer
	renderer := table.NewRenderer[testPerson](
		table.WithWriter[testPerson](&buf),
		table.WithHeaders[testPerson]("Name", "Status"),
		table.WithFormatter[testPerson]("Name", func(v any) any {
			return strings.ToUpper(v.(string))
		}),
	)

	person := testPerson{
		Name:   "Alice",
		Status: "active",
	}

	err := renderer.Append(person)
	g.Expect(err).ShouldNot(HaveOccurred())

	err = renderer.Render()
	g.Expect(err).ShouldNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).Should(ContainSubstring("ALICE"))
}

func TestRendererWithJQFormatter(t *testing.T) {
	g := NewWithT(t)

	var buf bytes.Buffer
	renderer := table.NewRenderer[testPersonWithTags](
		table.WithWriter[testPersonWithTags](&buf),
		table.WithHeaders[testPersonWithTags]("Name", "Tags"),
		table.WithFormatter[testPersonWithTags]("Tags", table.JQFormatter(`. | join(", ")`)),
	)

	person := testPersonWithTags{
		Name: "Alice",
		Tags: []string{"admin", "user"},
	}

	err := renderer.Append(person)
	g.Expect(err).ShouldNot(HaveOccurred())

	err = renderer.Render()
	g.Expect(err).ShouldNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).Should(ContainSubstring("Alice"))
	g.Expect(output).Should(ContainSubstring("admin, user"))
}

func TestRendererWithChainedFormatters(t *testing.T) {
	g := NewWithT(t)

	var buf bytes.Buffer
	renderer := table.NewRenderer[testPerson](
		table.WithWriter[testPerson](&buf),
		table.WithHeaders[testPerson]("Name", "Status"),
		table.WithFormatter[testPerson]("Name",
			table.ChainFormatters(
				table.JQFormatter("."),
				func(v any) any {
					return strings.ToUpper(v.(string))
				},
				func(v any) any {
					return "[" + v.(string) + "]"
				},
			),
		),
	)

	person := testPerson{
		Name:   "Alice",
		Status: "active",
	}

	err := renderer.Append(person)
	g.Expect(err).ShouldNot(HaveOccurred())

	err = renderer.Render()
	g.Expect(err).ShouldNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).Should(ContainSubstring("[ALICE]"))
}

func TestRendererWithJQExtraction(t *testing.T) {
	g := NewWithT(t)

	var buf bytes.Buffer
	renderer := table.NewRenderer[testPersonWithTags](
		table.WithWriter[testPersonWithTags](&buf),
		table.WithHeaders[testPersonWithTags]("Name", "Metadata"),
		table.WithFormatter[testPersonWithTags]("Metadata",
			table.JQFormatter(`.location // "Unknown"`),
		),
	)

	person := testPersonWithTags{
		Name:     "Alice",
		Metadata: map[string]any{"location": "NYC"},
	}

	err := renderer.Append(person)
	g.Expect(err).ShouldNot(HaveOccurred())

	err = renderer.Render()
	g.Expect(err).ShouldNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).Should(ContainSubstring("Alice"))
	g.Expect(output).Should(ContainSubstring("NYC"))
}

func TestRendererAppendAll(t *testing.T) {
	g := NewWithT(t)

	var buf bytes.Buffer
	renderer := table.NewRenderer[testPerson](
		table.WithWriter[testPerson](&buf),
		table.WithHeaders[testPerson]("Name", "Age"),
	)

	people := []testPerson{
		{Name: "Alice", Age: 30},
		{Name: "Bob", Age: 25},
		{Name: "Charlie", Age: 35},
	}

	err := renderer.AppendAll(people)
	g.Expect(err).ShouldNot(HaveOccurred())

	err = renderer.Render()
	g.Expect(err).ShouldNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).Should(ContainSubstring("Alice"))
	g.Expect(output).Should(ContainSubstring("Bob"))
	g.Expect(output).Should(ContainSubstring("Charlie"))
}

func TestRendererCaseInsensitiveMatching(t *testing.T) {
	g := NewWithT(t)

	var buf bytes.Buffer
	renderer := table.NewRenderer[testPerson](
		table.WithWriter[testPerson](&buf),
		table.WithHeaders[testPerson]("name", "AGE"),
	)

	person := testPerson{
		Name: "Alice",
		Age:  30,
	}

	err := renderer.Append(person)
	g.Expect(err).ShouldNot(HaveOccurred())

	err = renderer.Render()
	g.Expect(err).ShouldNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).Should(ContainSubstring("Alice"))
	g.Expect(output).Should(ContainSubstring("30"))
}

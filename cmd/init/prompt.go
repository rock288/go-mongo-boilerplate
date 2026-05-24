package main

import "github.com/charmbracelet/huh"

// Selection holds the answers a user gave the interactive form.
type Selection struct {
	Module string
	Keep   map[string]bool // keys: "kafka", "sqs", "observability", "samples"
}

// runPrompt shows the huh form and returns the user's selection.
// defaultModule is offered as the prefilled value for the module path field.
func runPrompt(defaultModule string) (Selection, error) {
	sel := Selection{
		Module: defaultModule,
		Keep:   map[string]bool{"kafka": true, "sqs": true, "observability": true, "samples": true},
	}

	keepKeys := []string{"kafka", "sqs", "observability", "samples"}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("New module path").
				Description("e.g. github.com/yourorg/your-service").
				Value(&sel.Module).
				Validate(validateModulePath),
		),
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Features to keep").
				Description("Toggle to KEEP; unchecked items are stripped").
				Options(
					huh.NewOption("Kafka (message broker)", "kafka").Selected(true),
					huh.NewOption("AWS SQS (second message broker)", "sqs").Selected(true),
					huh.NewOption("Observability (OTel + SigNoz)", "observability").Selected(true),
					huh.NewOption("Sample features (user, role)", "samples").Selected(true),
				).
				Value(&keepKeys),
		),
	)

	if err := form.Run(); err != nil {
		return sel, err
	}

	sel.Keep = map[string]bool{"kafka": false, "sqs": false, "observability": false, "samples": false}
	for _, k := range keepKeys {
		sel.Keep[k] = true
	}
	return sel, nil
}

// stripListFromSelection converts a Selection into the same shape that
// computeStripList produces (feature names to strip).
func stripListFromSelection(s Selection) []string {
	return computeStripList(!s.Keep["kafka"], !s.Keep["sqs"], !s.Keep["observability"], !s.Keep["samples"])
}

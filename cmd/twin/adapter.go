package main

// An adapter covers what one app needs beyond twin's generic isolation, such
// as settings it encrypts with the machine's ID. Adapters only ever run for
// the app they match: twin's isolation and data copy stay the same for every
// app, with no app listed by name.
type adapter struct {
	Name    string
	Matches func(App) bool
	// Steps join the clone, each right after the step titled After.
	Steps []adapterStep
	// Env adds variables to the twin's launcher.
	Env func(Plan) []string
	// Server starts the app's background server, which --service keeps
	// running; nil when it has none.
	Server []string
	// Rename runs the app through a link named after the twin, so tools
	// that find the original's processes by command line skip the twin's.
	Rename bool
	// Rows add lines to the finished twin's summary.
	Rows func(Plan) []string
}

type adapterStep struct {
	After string
	step
	// When limits the step to some plans; nil runs it always.
	When func(Plan) bool
}

// adapterFor returns the adapter for src, if one matches.
func adapterFor(src App) *adapter {
	for i := range adapters {
		if adapters[i].Matches(src) {
			return &adapters[i]
		}
	}
	return nil
}

// withAdapter inserts the matching adapter's steps into the platform's list.
func withAdapter(p Plan, list []step) []step {
	a := adapterFor(p.Src)
	if a == nil {
		return list
	}
	var out []step
	for _, s := range list {
		out = append(out, s)
		for _, as := range a.Steps {
			if as.After == s.Title && (as.When == nil || as.When(p)) {
				out = append(out, as.step)
			}
		}
	}
	return out
}

// adapterRows are the matching adapter's summary lines for a finished twin.
func adapterRows(p Plan) []string {
	if a := adapterFor(p.Src); a != nil && a.Rows != nil {
		return a.Rows(p)
	}
	return nil
}

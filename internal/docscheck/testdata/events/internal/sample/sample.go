// Package sample is a fixture of the docscheck tests. The go tool ignores a testdata
// directory, so this file never builds.
package sample

// EventSampleConstant is an event type that a constant holds.
const EventSampleConstant = "sample.constant"

// sampleHeader holds a string, and its name holds no "Event", so it is not an event type.
const sampleHeader = "X-Sample"

type recorder struct{}

func (recorder) emit(eventType, tailnetID, message string) {}

func (r recorder) run(eventType string) {
	r.emit("sample.literal", "", "")
	r.emit(eventType, "", "")
	r.emit(EventSampleConstant, "", "")
}

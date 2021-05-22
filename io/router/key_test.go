// SPDX-License-Identifier: Unlicense OR MIT

package router

import (
	"reflect"
	"testing"

	"github.com/l0k18/giocore/io/event"
	"github.com/l0k18/giocore/io/key"
	"github.com/l0k18/giocore/op"
)

func TestKeyWakeup(t *testing.T) {
	handler := new(int)
	var ops op.Ops
	key.InputOp{Tag: handler}.Add(&ops)

	var r Router
	// Test that merely adding a handler doesn't trigger redraw.
	r.Frame(&ops)
	if _, wake := r.WakeupTime(); wake {
		t.Errorf("adding key.InputOp triggered a redraw")
	}
	// However, adding a handler queues a Focus(false) event.
	if evts := r.Events(handler); len(evts) != 1 {
		t.Errorf("no Focus event for newly registered key.InputOp")
	}
	// Verify that r.Events does trigger a redraw.
	r.Frame(&ops)
	if _, wake := r.WakeupTime(); !wake {
		t.Errorf("key.FocusEvent event didn't trigger a redraw")
	}
}

func TestKeyMultiples(t *testing.T) {
	handlers := make([]int, 3)
	ops := new(op.Ops)
	r := new(Router)

	key.SoftKeyboardOp{Show: true}.Add(ops)
	key.InputOp{Tag: &handlers[0]}.Add(ops)
	key.FocusOp{Tag: &handlers[2]}.Add(ops)
	key.InputOp{Tag: &handlers[1]}.Add(ops)

	// The last one must be focused:
	key.InputOp{Tag: &handlers[2]}.Add(ops)

	r.Frame(ops)

	assertKeyEvent(t, r.Events(&handlers[0]), false)
	assertKeyEvent(t, r.Events(&handlers[1]), false)
	assertKeyEvent(t, r.Events(&handlers[2]), true)
	assertFocus(t, r, &handlers[2])
	assertKeyboard(t, r, TextInputOpen)
}

func TestKeyStacked(t *testing.T) {
	handlers := make([]int, 4)
	ops := new(op.Ops)
	r := new(Router)

	s := op.Save(ops)
	key.InputOp{Tag: &handlers[0]}.Add(ops)
	key.FocusOp{Tag: nil}.Add(ops)
	s.Load()
	s = op.Save(ops)
	key.SoftKeyboardOp{Show: false}.Add(ops)
	key.InputOp{Tag: &handlers[1]}.Add(ops)
	key.FocusOp{Tag: &handlers[1]}.Add(ops)
	s.Load()
	s = op.Save(ops)
	key.InputOp{Tag: &handlers[2]}.Add(ops)
	key.SoftKeyboardOp{Show: true}.Add(ops)
	s.Load()
	s = op.Save(ops)
	key.InputOp{Tag: &handlers[3]}.Add(ops)
	s.Load()

	r.Frame(ops)

	assertKeyEvent(t, r.Events(&handlers[0]), false)
	assertKeyEvent(t, r.Events(&handlers[1]), true)
	assertKeyEvent(t, r.Events(&handlers[2]), false)
	assertKeyEvent(t, r.Events(&handlers[3]), false)
	assertFocus(t, r, &handlers[1])
	assertKeyboard(t, r, TextInputOpen)
}

func TestKeySoftKeyboardNoFocus(t *testing.T) {
	ops := new(op.Ops)
	r := new(Router)

	// It's possible to open the keyboard
	// without any active focus:
	key.SoftKeyboardOp{Show: true}.Add(ops)

	r.Frame(ops)

	assertFocus(t, r, nil)
	assertKeyboard(t, r, TextInputOpen)
}

func TestKeyRemoveFocus(t *testing.T) {
	handlers := make([]int, 2)
	ops := new(op.Ops)
	r := new(Router)

	// New InputOp with Focus and Keyboard:
	s := op.Save(ops)
	key.InputOp{Tag: &handlers[0]}.Add(ops)
	key.FocusOp{Tag: &handlers[0]}.Add(ops)
	key.SoftKeyboardOp{Show: true}.Add(ops)
	s.Load()

	// New InputOp without any focus:
	s = op.Save(ops)
	key.InputOp{Tag: &handlers[1]}.Add(ops)
	s.Load()

	r.Frame(ops)

	// Add some key events:
	event := event.Event(key.Event{Name: key.NameTab, Modifiers: key.ModShortcut, State: key.Press})
	r.Queue(event)

	assertKeyEvent(t, r.Events(&handlers[0]), true, event)
	assertKeyEvent(t, r.Events(&handlers[1]), false)
	assertFocus(t, r, &handlers[0])
	assertKeyboard(t, r, TextInputOpen)

	ops.Reset()

	// Will get the focus removed:
	s = op.Save(ops)
	key.InputOp{Tag: &handlers[0]}.Add(ops)
	s.Load()

	// Unchanged:
	s = op.Save(ops)
	key.InputOp{Tag: &handlers[1]}.Add(ops)
	s.Load()

	// Remove focus by focusing on a tag that don't exist.
	s = op.Save(ops)
	key.FocusOp{Tag: new(int)}.Add(ops)
	s.Load()

	r.Frame(ops)

	assertKeyEventUnexpected(t, r.Events(&handlers[1]))
	assertFocus(t, r, nil)
	assertKeyboard(t, r, TextInputClose)

	ops.Reset()

	s = op.Save(ops)
	key.InputOp{Tag: &handlers[0]}.Add(ops)
	s.Load()

	s = op.Save(ops)
	key.InputOp{Tag: &handlers[1]}.Add(ops)
	s.Load()

	r.Frame(ops)

	assertKeyEventUnexpected(t, r.Events(&handlers[0]))
	assertKeyEventUnexpected(t, r.Events(&handlers[1]))
	assertFocus(t, r, nil)
	assertKeyboard(t, r, TextInputKeep)

	ops.Reset()

	// Set focus to InputOp which already
	// exists in the previous frame:
	s = op.Save(ops)
	key.FocusOp{Tag: &handlers[0]}.Add(ops)
	key.InputOp{Tag: &handlers[0]}.Add(ops)
	key.SoftKeyboardOp{Show: true}.Add(ops)
	s.Load()

	// Remove focus.
	s = op.Save(ops)
	key.InputOp{Tag: &handlers[1]}.Add(ops)
	key.FocusOp{Tag: nil}.Add(ops)
	s.Load()

	r.Frame(ops)

	assertKeyEventUnexpected(t, r.Events(&handlers[1]))
	assertFocus(t, r, nil)
	assertKeyboard(t, r, TextInputOpen)
}

func TestKeyFocusedInvisible(t *testing.T) {
	handlers := make([]int, 2)
	ops := new(op.Ops)
	r := new(Router)

	// Set new InputOp with focus:
	s := op.Save(ops)
	key.FocusOp{Tag: &handlers[0]}.Add(ops)
	key.InputOp{Tag: &handlers[0]}.Add(ops)
	key.SoftKeyboardOp{Show: true}.Add(ops)
	s.Load()

	// Set new InputOp without focus:
	s = op.Save(ops)
	key.InputOp{Tag: &handlers[1]}.Add(ops)
	s.Load()

	r.Frame(ops)

	assertKeyEvent(t, r.Events(&handlers[0]), true)
	assertKeyEvent(t, r.Events(&handlers[1]), false)
	assertFocus(t, r, &handlers[0])
	assertKeyboard(t, r, TextInputOpen)

	ops.Reset()

	//
	// Removed first (focused) element!
	//

	// Unchanged:
	s = op.Save(ops)
	key.InputOp{Tag: &handlers[1]}.Add(ops)
	s.Load()

	r.Frame(ops)

	assertKeyEventUnexpected(t, r.Events(&handlers[0]))
	assertKeyEventUnexpected(t, r.Events(&handlers[1]))
	assertFocus(t, r, nil)
	assertKeyboard(t, r, TextInputClose)

	ops.Reset()

	// Respawn the first element:
	// It must receive one `Event{Focus: false}`.
	s = op.Save(ops)
	key.InputOp{Tag: &handlers[0]}.Add(ops)
	s.Load()

	// Unchanged
	s = op.Save(ops)
	key.InputOp{Tag: &handlers[1]}.Add(ops)
	s.Load()

	r.Frame(ops)

	assertKeyEvent(t, r.Events(&handlers[0]), false)
	assertKeyEventUnexpected(t, r.Events(&handlers[1]))
	assertFocus(t, r, nil)
	assertKeyboard(t, r, TextInputKeep)

}

func assertKeyEvent(t *testing.T, events []event.Event, expected bool, expectedInputs ...event.Event) {
	t.Helper()
	var evtFocus int
	var evtKeyPress int
	for _, e := range events {
		switch ev := e.(type) {
		case key.FocusEvent:
			if ev.Focus != expected {
				t.Errorf("focus is expected to be %v, got %v", expected, ev.Focus)
			}
			evtFocus++
		case key.Event, key.EditEvent:
			if len(expectedInputs) <= evtKeyPress {
				t.Errorf("unexpected key events")
			}
			if !reflect.DeepEqual(ev, expectedInputs[evtKeyPress]) {
				t.Errorf("expected %v events, got %v", expectedInputs[evtKeyPress], ev)
			}
			evtKeyPress++
		}
	}
	if evtFocus <= 0 {
		t.Errorf("expected focus event")
	}
	if evtFocus > 1 {
		t.Errorf("expected single focus event")
	}
	if evtKeyPress != len(expectedInputs) {
		t.Errorf("expected key events")
	}
}

func assertKeyEventUnexpected(t *testing.T, events []event.Event) {
	t.Helper()
	var evtFocus int
	for _, e := range events {
		switch e.(type) {
		case key.FocusEvent:
			evtFocus++
		}
	}
	if evtFocus > 1 {
		t.Errorf("unexpected focus event")
	}
}

func assertFocus(t *testing.T, router *Router, expected event.Tag) {
	t.Helper()
	if router.kqueue.focus != expected {
		t.Errorf("expected %v to be focused, got %v", expected, router.kqueue.focus)
	}
}

func assertKeyboard(t *testing.T, router *Router, expected TextInputState) {
	t.Helper()
	if router.kqueue.state != expected {
		t.Errorf("expected %v keyboard, got %v", expected, router.kqueue.state)
	}
}

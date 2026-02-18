package statemachine

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type TestStruct struct {
	Id     string
	Status string
}

func (p *TestStruct) SetState(s State) {
	p.Status = string(s)
}

func (p *TestStruct) GetState() State {
	return State(p.Status)
}

type onMelt struct{}

func (receiver *onMelt) GetEvent() Event {
	return "onMelt"
}

type onVapourise struct{}

func (receiver *onVapourise) GetEvent() Event {
	return "onVapourise"
}

type onCondensation struct{}

func (receiver *onCondensation) GetEvent() Event {
	return "onCondensation"
}

type onFreeze struct{}

func (receiver *onFreeze) GetEvent() Event {
	return "onFreeze"
}

// testEvent implements TransitionEvent for tests.
type testEvent string

func (e testEvent) GetEvent() Event { return Event(e) }

func getTestData() (EventKey, map[EventKey]Transition) {
	sm := NewStatemachine(EventKey{Src: "SOLID", Event: "onMelt"})
	sm.AddTransition(Transition{
		Src:        []State{"SOLID"},
		Event:      "onMelt",
		Dst:        "LIQUID",
		Transition: onEvent(),
	})
	sm.AddTransition(Transition{
		Src:              []State{"LIQUID"},
		Event:            "onVapourise",
		Dst:              "GAS",
		BeforeTransition: onBeforeEvent(),
		Transition:       onEvent(),
		AfterTransition:  onAfterEvent(),
		OnSuccess:        onSuccess(),
		OnFailure:        onFailure(),
	})
	sm.AddTransition(Transition{
		Src:              []State{"GAS"},
		Event:            "onCondensation",
		Dst:              "LIQUID",
		BeforeTransition: onBeforeEvent(),
		Transition:       onEvent(),
		AfterTransition:  onAfterEvent(),
		OnSuccess:        onSuccess(),
		OnFailure:        onFailure(),
	})
	sm.AddTransition(Transition{
		Src:              []State{"LIQUID"},
		Event:            "onFreeze",
		Dst:              "SOLID",
		BeforeTransition: onBeforeEvent(),
		Transition:       onEvent(),
		AfterTransition:  onAfterEvent(),
		OnSuccess:        onSuccess(),
		OnFailure:        onFailure(),
	})
	return sm.GetTransitions()
}

func onBeforeEvent() BeforeTransitionHandler {
	return func(context.Context, TransitionModel) (TransitionModel, error) {
		fmt.Println("before")
		return nil, nil
	}
}

func onEvent() TransitionHandler {
	return func(context.Context, TransitionEvent, TransitionModel) (TransitionModel, error) {
		fmt.Println("during")
		return nil, nil
	}
}

func onAfterEvent() AfterTransitionHandler {
	return func(context.Context, TransitionModel) (TransitionModel, error) {
		fmt.Println("after")
		return nil, nil
	}
}

func onSuccess() OnSuccessHandler {
	return func(context.Context, TransitionModel) (TransitionModel, error) {
		fmt.Println("success")
		return nil, nil
	}
}

func onFailure() OnFailureHandler {
	return func(ctx context.Context, t TransitionModel, s Error, err error) (TransitionModel, error) {
		fmt.Println("failure : ", err)
		return nil, err
	}
}

func Test_stateMachine_TriggerTransition(t *testing.T) {
	ek, trs := getTestData()
	tests := []struct {
		name    string
		ctx     context.Context
		e       TransitionEvent
		model   TransitionModel
		wantErr bool
	}{
		{
			name:    "SOLID -> onMelt",
			ctx:     context.Background(),
			e:       &onMelt{},
			model:   &TestStruct{Id: "t_123", Status: "SOLID"},
			wantErr: false,
		},
		{
			name:    "LIQUID -> onVapourise",
			ctx:     context.Background(),
			e:       &onVapourise{},
			model:   &TestStruct{Id: "t_123", Status: "LIQUID"},
			wantErr: false,
		},
		{
			name:    "GAS -> onCondensation",
			ctx:     context.Background(),
			e:       &onCondensation{},
			model:   &TestStruct{Id: "t_123", Status: "GAS"},
			wantErr: false,
		},
		{
			name:    "LIQUID -> onFreeze",
			ctx:     context.Background(),
			e:       &onFreeze{},
			model:   &TestStruct{Id: "t_123", Status: "LIQUID"},
			wantErr: false,
		},
		{
			name:    "LIQUID -> onMelt (undefined)",
			ctx:     context.Background(),
			e:       &onMelt{},
			model:   &TestStruct{Id: "t_123", Status: "LIQUID"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &stateMachine{startEvent: ek, transitions: trs, logger: nil}
			_, err := s.TriggerTransition(tt.ctx, tt.e, tt.model)
			if (err != nil) != tt.wantErr {
				t.Errorf("TriggerTransition() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTriggerTransition_NilModel(t *testing.T) {
	sm := NewStatemachine(EventKey{Src: "A", Event: "e"})
	sm.AddTransition(Transition{
		Src: []State{"A"}, Event: "e", Dst: "B",
	})
	_, err := sm.TriggerTransition(context.Background(), &onMelt{}, nil)
	if err == nil {
		t.Fatal("expected error for nil model")
	}
	if !errors.Is(err, ErrNilModel) {
		t.Errorf("expected ErrNilModel, got %v", err)
	}
}

func TestTriggerTransition_UndefinedTransition(t *testing.T) {
	sm := NewStatemachine(EventKey{Src: "SOLID", Event: "onMelt"})
	sm.AddTransition(Transition{
		Src: []State{"SOLID"}, Event: "onMelt", Dst: "LIQUID",
	})
	model := &TestStruct{Status: "LIQUID"}
	_, err := sm.TriggerTransition(context.Background(), &onMelt{}, model)
	if err == nil {
		t.Fatal("expected error for undefined transition")
	}
	if !errors.Is(err, ErrUndefinedTransition) {
		t.Errorf("expected ErrUndefinedTransition, got %v", err)
	}
	if model.GetState() != "LIQUID" {
		t.Errorf("model state should be unchanged, got %v", model.GetState())
	}
}

func TestTriggerTransition_ErrIgnoreAbortsTransition(t *testing.T) {
	sm := NewStatemachine(EventKey{Src: "A", Event: "e"})
	sm.AddTransition(Transition{
		Src:        []State{"A"},
		Event:      "e",
		Dst:        "B",
		Transition: func(ctx context.Context, _ TransitionEvent, m TransitionModel) (TransitionModel, error) {
			return nil, errors.New("fail")
		},
		OnFailure: func(ctx context.Context, m TransitionModel, _ Error, err error) (TransitionModel, error) {
			return m, ErrIgnore
		},
	})
	model := &TestStruct{Status: "A"}
	out, err := sm.TriggerTransition(context.Background(), testEvent("e"), model)
	if err != nil {
		t.Fatalf("ErrIgnore should swallow error: %v", err)
	}
	if out != model {
		t.Error("expected same model returned")
	}
	if model.GetState() != "A" {
		t.Errorf("state should be unchanged after ErrIgnore, got %v", model.GetState())
	}
}

func TestTriggerTransition_HandlerReturnValueUsed(t *testing.T) {
	sm := NewStatemachine(EventKey{Src: "A", Event: "e"})
	sm.AddTransition(Transition{
		Src:        []State{"A"},
		Event:      "e",
		Dst:        "B",
		BeforeTransition: func(ctx context.Context, m TransitionModel) (TransitionModel, error) {
			m.SetState("B")
			return m, nil
		},
	})
	model := &TestStruct{Id: "x", Status: "A"}
	out, err := sm.TriggerTransition(context.Background(), testEvent("e"), model)
	if err != nil {
		t.Fatal(err)
	}
	if out.GetState() != "B" {
		t.Errorf("expected state B, got %v", out.GetState())
	}
}

func TestAddTransition_Uninitialized(t *testing.T) {
	s := &stateMachine{transitions: nil}
	err := s.AddTransition(Transition{Src: []State{"A"}, Event: "e", Dst: "B"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrUninitializedSM) {
		t.Errorf("expected ErrUninitializedSM, got %v", err)
	}
}

func TestGetTransitions_ReturnsCopy(t *testing.T) {
	sm := NewStatemachine(EventKey{Src: "A", Event: "e"})
	sm.AddTransition(Transition{Src: []State{"A"}, Event: "e", Dst: "B"})
	_, m := sm.GetTransitions()
	if m == nil {
		t.Fatal("expected non-nil map")
	}
	// Mutate the returned map; should not affect the state machine
	for k := range m {
		delete(m, k)
		break
	}
	model := &TestStruct{Status: "A"}
	_, err := sm.TriggerTransition(context.Background(), testEvent("e"), model)
	if err != nil {
		t.Fatalf("mutating returned map should not break machine: %v", err)
	}
	if model.GetState() != "B" {
		t.Errorf("expected state B, got %v", model.GetState())
	}
}

func TestVisualize_NilAndEmpty(t *testing.T) {
	// Should not panic
	Visualize(nil)
	sm := NewStatemachine(EventKey{Src: "A", Event: "e"})
	Visualize(sm) // empty transitions
}

func TestNewStatemachineWithOptions_WithLogger(t *testing.T) {
	var logged bool
	sm := NewStatemachineWithOptions(EventKey{Src: "A", Event: "e"}, WithLogger(LoggerFunc(func(tr Transition) {
		logged = true
	})))
	sm.AddTransition(Transition{Src: []State{"A"}, Event: "e", Dst: "B"})
	_, _ = sm.TriggerTransition(context.Background(), testEvent("e"), &TestStruct{Status: "A"})
	if !logged {
		t.Error("custom logger should have been called")
	}
}

// LoggerFunc adapts a function to the Logger interface.
type LoggerFunc func(Transition)

func (f LoggerFunc) LogTransition(tr Transition) { f(tr) }

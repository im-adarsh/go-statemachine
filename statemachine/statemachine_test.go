package statemachine

import (
	"context"
	"errors"
	"fmt"
	"sync"
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

func getTestData() (EventKey, map[EventKey][]Transition) {
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
			s := &stateMachine{startEvent: ek, transitions: trs, stateEntries: make(map[State][]stateEntryHandlerWithConcurrency), stateExits: make(map[State][]stateExitHandlerWithConcurrency), logger: nil}
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

func TestConditionalFlow_MultipleTransitions(t *testing.T) {
	sm := NewStatemachine(EventKey{Src: "START", Event: "process"})
	
	// Add multiple transitions with guards (conditional flow)
	sm.AddTransition(Transition{
		Src: []State{"START"}, Event: "process", Dst: "HIGH",
		Guard: func(ctx context.Context, _ TransitionEvent, m TransitionModel) (bool, error) {
			return m.(*TestStruct).Id == "high", nil
		},
	})
	sm.AddTransition(Transition{
		Src: []State{"START"}, Event: "process", Dst: "MEDIUM",
		Guard: func(ctx context.Context, _ TransitionEvent, m TransitionModel) (bool, error) {
			return m.(*TestStruct).Id == "medium", nil
		},
	})
	sm.AddTransition(Transition{
		Src: []State{"START"}, Event: "process", Dst: "LOW",
		// No guard = default/fallback
	})

	// Test HIGH path
	model1 := &TestStruct{Id: "high", Status: "START"}
	_, err := sm.TriggerTransition(context.Background(), testEvent("process"), model1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model1.GetState() != "HIGH" {
		t.Errorf("expected HIGH, got %v", model1.GetState())
	}

	// Test MEDIUM path
	model2 := &TestStruct{Id: "medium", Status: "START"}
	_, err = sm.TriggerTransition(context.Background(), testEvent("process"), model2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model2.GetState() != "MEDIUM" {
		t.Errorf("expected MEDIUM, got %v", model2.GetState())
	}

	// Test LOW path (fallback)
	model3 := &TestStruct{Id: "other", Status: "START"}
	_, err = sm.TriggerTransition(context.Background(), testEvent("process"), model3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model3.GetState() != "LOW" {
		t.Errorf("expected LOW, got %v", model3.GetState())
	}
}

func TestConcurrentStateHandlers(t *testing.T) {
	sm := NewStatemachine(EventKey{Src: "A", Event: "e"})
	sm.AddTransition(Transition{Src: []State{"A"}, Event: "e", Dst: "B"})

	var callOrder []string
	var mu sync.Mutex

	sm.AddStateEntry("B", func(ctx context.Context, m TransitionModel) error {
		mu.Lock()
		callOrder = append(callOrder, "seq1")
		mu.Unlock()
		return nil
	})

	sm.AddStateEntryConcurrent("B", func(ctx context.Context, m TransitionModel) error {
		mu.Lock()
		callOrder = append(callOrder, "concurrent1")
		mu.Unlock()
		return nil
	})

	sm.AddStateEntryConcurrent("B", func(ctx context.Context, m TransitionModel) error {
		mu.Lock()
		callOrder = append(callOrder, "concurrent2")
		mu.Unlock()
		return nil
	})

	model := &TestStruct{Status: "A"}
	_, err := sm.TriggerTransition(context.Background(), testEvent("e"), model)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Sequential handler should run first
	if len(callOrder) < 3 {
		t.Errorf("expected at least 3 handlers, got %d", len(callOrder))
	}
	if callOrder[0] != "seq1" {
		t.Errorf("expected seq1 first, got %s", callOrder[0])
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

func TestGuardCondition_BlocksTransition(t *testing.T) {
	sm := NewStatemachine(EventKey{Src: "A", Event: "e"})
	sm.AddTransition(Transition{
		Src: []State{"A"}, Event: "e", Dst: "B",
		Guard: func(ctx context.Context, _ TransitionEvent, m TransitionModel) (bool, error) {
			// Only allow if status contains "allow"
			return m.GetState() == "A" && m.(*TestStruct).Id == "allow", nil
		},
	})

	// Guard fails
	model1 := &TestStruct{Id: "deny", Status: "A"}
	_, err := sm.TriggerTransition(context.Background(), testEvent("e"), model1)
	if err == nil {
		t.Fatal("expected error when guard fails")
	}
	if !errors.Is(err, ErrGuardFailed) {
		t.Errorf("expected ErrGuardFailed, got %v", err)
	}
	if model1.GetState() != "A" {
		t.Errorf("state should be unchanged, got %v", model1.GetState())
	}

	// Guard passes
	model2 := &TestStruct{Id: "allow", Status: "A"}
	_, err = sm.TriggerTransition(context.Background(), testEvent("e"), model2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model2.GetState() != "B" {
		t.Errorf("expected state B, got %v", model2.GetState())
	}
}

func TestGuardCondition_WithOnFailure(t *testing.T) {
	sm := NewStatemachine(EventKey{Src: "A", Event: "e"})
	// Add a fallback transition without guard so OnFailure can be called
	sm.AddTransition(Transition{
		Src: []State{"A"}, Event: "e", Dst: "B",
		Guard: func(ctx context.Context, _ TransitionEvent, m TransitionModel) (bool, error) {
			return false, nil // Guard fails
		},
		OnFailure: func(ctx context.Context, m TransitionModel, code Error, err error) (TransitionModel, error) {
			return m, ErrIgnore // Swallow error
		},
	})
	// Add fallback transition
	sm.AddTransition(Transition{
		Src: []State{"A"}, Event: "e", Dst: "FALLBACK",
	})

	model := &TestStruct{Status: "A"}
	_, err := sm.TriggerTransition(context.Background(), testEvent("e"), model)
	// With conditional flow, the fallback transition will match, so we won't hit OnFailure
	// But the guard still works - first transition guard fails, so fallback is used
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.GetState() != "FALLBACK" {
		t.Errorf("expected FALLBACK (fallback transition), got %v", model.GetState())
	}
}

func TestStateEntryExit_Callbacks(t *testing.T) {
	var entryCalled, exitCalled bool
	var entryState, exitState State

	sm := NewStatemachine(EventKey{Src: "A", Event: "e"})
	sm.AddTransition(Transition{Src: []State{"A"}, Event: "e", Dst: "B"})

	sm.AddStateExit("A", func(ctx context.Context, m TransitionModel) error {
		exitCalled = true
		exitState = m.GetState()
		return nil
	})

	sm.AddStateEntry("B", func(ctx context.Context, m TransitionModel) error {
		entryCalled = true
		entryState = m.GetState()
		return nil
	})

	model := &TestStruct{Status: "A"}
	_, err := sm.TriggerTransition(context.Background(), testEvent("e"), model)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !exitCalled {
		t.Error("state exit handler should have been called")
	}
	if exitState != "A" {
		t.Errorf("exit handler should see state A, got %v", exitState)
	}

	if !entryCalled {
		t.Error("state entry handler should have been called")
	}
	if entryState != "B" {
		t.Errorf("entry handler should see state B, got %v", entryState)
	}
}

func TestStateEntryExit_ErrorAbortsTransition(t *testing.T) {
	sm := NewStatemachine(EventKey{Src: "A", Event: "e"})
	sm.AddTransition(Transition{Src: []State{"A"}, Event: "e", Dst: "B"})

	sm.AddStateExit("A", func(ctx context.Context, m TransitionModel) error {
		return errors.New("exit failed")
	})

	model := &TestStruct{Status: "A"}
	_, err := sm.TriggerTransition(context.Background(), testEvent("e"), model)
	if err == nil {
		t.Fatal("expected error from exit handler")
	}
	// State should not change if exit handler fails
	if model.GetState() != "A" {
		t.Errorf("state should remain A after exit failure, got %v", model.GetState())
	}
}

func TestConcurrentAccess_ThreadSafe(t *testing.T) {
	sm := NewStatemachine(EventKey{Src: "A", Event: "e"})
	sm.AddTransition(Transition{Src: []State{"A"}, Event: "e", Dst: "B"})
	sm.AddTransition(Transition{Src: []State{"B"}, Event: "e2", Dst: "A"})

	const goroutines = 10
	const iterations = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				model := &TestStruct{Status: "A"}
				_, err := sm.TriggerTransition(context.Background(), testEvent("e"), model)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if model.GetState() != "B" {
					t.Errorf("expected state B, got %v", model.GetState())
					return
				}
			}
		}()
	}

	wg.Wait()
}

func TestConcurrentAddTransition_ThreadSafe(t *testing.T) {
	sm := NewStatemachine(EventKey{Src: "A", Event: "e"})

	const goroutines = 5
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			err := sm.AddTransition(Transition{
				Src: []State{State(fmt.Sprintf("S%d", id))}, Event: "e", Dst: "D",
			})
			if err != nil && !errors.Is(err, ErrDuplicateTransition) {
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}

	wg.Wait()
}

func TestConditionBlock_IfElse(t *testing.T) {
	sm := NewStatemachine(EventKey{Src: "START", Event: "process"})

	err := sm.AddConditionBlock(ConditionBlock{
		Src:   []State{"START"},
		Event: "process",
		Cases: []ConditionCase{
			{
				Condition: func(ctx context.Context, _ TransitionEvent, m TransitionModel) (bool, error) {
					return m.(*TestStruct).Id == "high", nil
				},
				Transition: Transition{Src: []State{"START"}, Dst: "HIGH"},
			},
			{
				Condition: func(ctx context.Context, _ TransitionEvent, m TransitionModel) (bool, error) {
					return m.(*TestStruct).Id == "medium", nil
				},
				Transition: Transition{Src: []State{"START"}, Dst: "MEDIUM"},
			},
		},
		ElseTransition: &Transition{Src: []State{"START"}, Dst: "LOW"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	model1 := &TestStruct{Id: "high", Status: "START"}
	_, err = sm.TriggerTransition(context.Background(), testEvent("process"), model1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model1.GetState() != "HIGH" {
		t.Errorf("expected HIGH, got %v", model1.GetState())
	}

	model2 := &TestStruct{Id: "medium", Status: "START"}
	_, err = sm.TriggerTransition(context.Background(), testEvent("process"), model2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model2.GetState() != "MEDIUM" {
		t.Errorf("expected MEDIUM, got %v", model2.GetState())
	}

	model3 := &TestStruct{Id: "low", Status: "START"}
	_, err = sm.TriggerTransition(context.Background(), testEvent("process"), model3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model3.GetState() != "LOW" {
		t.Errorf("expected LOW, got %v", model3.GetState())
	}
}

func TestSwitchBlock_SwitchCase(t *testing.T) {
	sm := NewStatemachine(EventKey{Src: "START", Event: "route"})

	err := sm.AddSwitchBlock(SwitchBlock{
		Src:   []State{"START"},
		Event: "route",
		SwitchExpr: func(ctx context.Context, _ TransitionEvent, m TransitionModel) (interface{}, error) {
			return m.(*TestStruct).Id, nil
		},
		Cases: []SwitchCase{
			{Value: "A", Transition: Transition{Src: []State{"START"}, Dst: "STATE_A"}},
			{Value: "B", Transition: Transition{Src: []State{"START"}, Dst: "STATE_B"}},
			{Value: "C", Transition: Transition{Src: []State{"START"}, Dst: "STATE_C"}},
		},
		DefaultTransition: &Transition{Src: []State{"START"}, Dst: "DEFAULT"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	model1 := &TestStruct{Id: "A", Status: "START"}
	_, err = sm.TriggerTransition(context.Background(), testEvent("route"), model1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model1.GetState() != "STATE_A" {
		t.Errorf("expected STATE_A, got %v", model1.GetState())
	}

	model2 := &TestStruct{Id: "X", Status: "START"}
	_, err = sm.TriggerTransition(context.Background(), testEvent("route"), model2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model2.GetState() != "DEFAULT" {
		t.Errorf("expected DEFAULT, got %v", model2.GetState())
	}
}
